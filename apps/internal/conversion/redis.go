package conversion

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// lastStatusTTL is long on purpose: §45 notes networks re-send postbacks
// with hours-to-days delay, and the progression check has to still be
// correct when they do.
const lastStatusTTL = 30 * 24 * time.Hour

// RedisStore wraps a durable Store with a read-through Redis cache for the
// progression check ("the last seen status per click_id lives next to the
// dedup keys... so the check costs one lookup on a path that is already
// doing one" — §45). It never becomes the arbiter of correctness: dedup
// itself stays on inner's atomic Postgres constraint (see conversion.go's
// package doc for why), and the cache is only ever written AFTER inner
// confirms a success — never before — so a cache write that never happens
// can only make a later check fall through to Postgres, not falsely report
// "already seen."
//
// REDIS UNAVAILABLE: every Redis call here falls through to inner on any
// error, per §45's explicit rule — a missing progression check is a wrong
// report; a refused postback is a lost conversion, and losing a conversion
// is the worse failure.
type RedisStore struct {
	inner  Store
	rdb    *redis.Client
	logger *slog.Logger
}

func NewRedisStore(inner Store, rdb *redis.Client, logger *slog.Logger) *RedisStore {
	return &RedisStore{inner: inner, rdb: rdb, logger: logger}
}

var _ Store = (*RedisStore)(nil)

func (s *RedisStore) LastStatus(ctx context.Context, organizationID, clickID string) (event.Type, bool, error) {
	key := lastStatusKey(organizationID, clickID)

	val, err := s.rdb.Get(ctx, key).Result()
	if err == nil {
		return event.Type(val), true, nil
	}
	if !errors.Is(err, redis.Nil) {
		s.logger.Warn("redis unavailable for progression check, falling through to durable store",
			"error", err, "organization_id", organizationID, "click_id", clickID)
	}

	status, ok, err := s.inner.LastStatus(ctx, organizationID, clickID)
	if err != nil {
		return "", false, err
	}
	if ok {
		// Best-effort repopulate. Redis is cache only (STACK: "sticky
		// CACHE only, postback dedup") — a failed Set here just means the
		// next lookup pays the same Postgres round trip again.
		if setErr := s.rdb.Set(ctx, key, string(status), lastStatusTTL).Err(); setErr != nil {
			s.logger.Warn("redis unavailable, could not cache last status",
				"error", setErr, "organization_id", organizationID, "click_id", clickID)
		}
	}
	return status, ok, nil
}

func (s *RedisStore) Record(ctx context.Context, e Entry) (string, ResultKind, error) {
	id, actual, err := s.inner.Record(ctx, e)
	if err != nil {
		return "", "", err
	}
	if actual == ResultSuccess {
		key := lastStatusKey(e.OrganizationID, e.ClickID)
		if setErr := s.rdb.Set(ctx, key, string(e.Status), lastStatusTTL).Err(); setErr != nil {
			s.logger.Warn("redis unavailable, could not cache newly recorded status",
				"error", setErr, "organization_id", e.OrganizationID, "click_id", e.ClickID)
		}
	}
	return id, actual, nil
}

func lastStatusKey(organizationID, clickID string) string {
	return "flox:conversion:last-status:" + organizationID + ":" + clickID
}
