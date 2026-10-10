-- +goose NO TRANSACTION

-- Tech audit 2026-10-06, DB5: internal/retention deletes expired sessions
-- in batches; this index is what it finds them by instead of scanning
-- every session. CONCURRENTLY, as 00061: no write lock on sessions (every
-- sign-in writes one) while it builds.

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS sessions_expires_at_idx;
