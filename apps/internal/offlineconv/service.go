package offlineconv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
	"github.com/ismagilovnail/flox/apps/internal/adclick"
	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

const (
	// maxAttempts: a conversion that failed this many times for a reason
	// other than a refusal (rate limits, Google down, access missing) is
	// given up — about two days with retryDelay's growth.
	maxAttempts = 12
	// candidatesPerRun / claimPerRun bound one run's work per organization
	// and in all.
	candidatesPerRun = 1000
	claimPerRun      = 5000
)

// retryDelay grows from a minute, doubling, to six hours.
func retryDelay(attempts int) time.Duration {
	d := time.Minute << min(attempts, 9)
	return min(d, 6*time.Hour)
}

// Ingester is what sends events to Google (googleads.DataManager).
type Ingester interface {
	Ingest(ctx context.Context, creds adaccount.Credentials, dest googleads.Destination, events []googleads.Event, validateOnly bool) (googleads.IngestResult, error)
}

// Credentials reads a source's decrypted credentials (adaccount.Repository).
type Credentials interface {
	CredentialsByTrafficSourceID(ctx context.Context, orgID, trafficSourceID string) (adaccount.Credentials, error)
}

type Service struct {
	repo   *Repository
	clicks attribution.ClickResolver
	creds  Credentials
	ingest Ingester
	// metrica: WithMetrica (metrica.go).
	metrica Metrica
	logger  *slog.Logger
	newID   func() string
	now     func() time.Time
}

// NewService: clicks, creds and ingest are the worker's; the API's
// settings routes need only repo (the others may be nil there).
func NewService(repo *Repository, clicks attribution.ClickResolver, creds Credentials, ingest Ingester, logger *slog.Logger) *Service {
	return &Service{repo: repo, clicks: clicks, creds: creds, ingest: ingest, logger: logger, newID: idgen.New, now: time.Now}
}

// --- settings (HTTP) ---

var (
	actionIDPattern  = regexp.MustCompile(`^[0-9]{1,20}$`)
	accountIDPattern = regexp.MustCompile(`^[0-9]{3}-?[0-9]{3}-?[0-9]{4}$`)
)

func (s *Service) Get(ctx context.Context, orgID, trafficSourceID string) (Status, error) {
	c, err := s.repo.googleConnection(ctx, orgID, trafficSourceID)
	if err != nil {
		return Status{}, err
	}
	return s.repo.status(ctx, orgID, c)
}

// Update validates and writes in. Turning the export on needs at least one
// conversion action; only conversions recorded from then on are sent.
func (s *Service) Update(ctx context.Context, orgID, trafficSourceID string, in Settings) (Status, error) {
	c, err := s.repo.googleConnection(ctx, orgID, trafficSourceID)
	if err != nil {
		return Status{}, err
	}
	in.LoginAccountID = strings.TrimSpace(in.LoginAccountID)
	in.LeadActionID = strings.TrimSpace(in.LeadActionID)
	in.PurchaseActionID = strings.TrimSpace(in.PurchaseActionID)
	in.RepurchaseActionID = strings.TrimSpace(in.RepurchaseActionID)

	fields := map[string]string{}
	for name, v := range map[string]string{
		"leadActionId": in.LeadActionID, "purchaseActionId": in.PurchaseActionID, "repurchaseActionId": in.RepurchaseActionID,
	} {
		if v != "" && !actionIDPattern.MatchString(v) {
			fields[name] = "a Google Ads conversion action id: digits only"
		}
	}
	if in.LoginAccountID != "" && !accountIDPattern.MatchString(in.LoginAccountID) {
		fields["loginAccountId"] = "a Google Ads account id: 10 digits, dashes allowed"
	}
	if in.Enabled && in.LeadActionID == "" && in.PurchaseActionID == "" && in.RepurchaseActionID == "" {
		fields["purchaseActionId"] = "name at least one conversion action to turn the export on"
	}
	if len(fields) > 0 {
		return Status{}, apierror.Validation("invalid offline conversion settings", fields)
	}
	if err := s.repo.setSettings(ctx, orgID, c.ID, in); err != nil {
		return Status{}, err
	}
	return s.Get(ctx, orgID, trafficSourceID)
}

// --- the worker's ---

// actionFor is the conversion action a status is reported as; empty for none.
func actionFor(c connection, status string) string {
	switch event.Type(status) {
	case event.CpaHold:
		return c.LeadActionID
	case event.CpaAccept:
		return c.PurchaseActionID
	case event.CpaRedep:
		return c.RepurchaseActionID
	}
	return ""
}

// Enqueue looks at every new conversion of every organization with the
// export on, and puts refunded purchases back to be restated.
func (s *Service) Enqueue(ctx context.Context) error {
	orgs, err := s.repo.exportingOrgs(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, o := range orgs {
		if err := s.enqueueOrg(ctx, o); err != nil {
			errs = append(errs, fmt.Errorf("organization %s: %w", o.OrganizationID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) enqueueOrg(ctx context.Context, o exportingOrg) error {
	cands, err := s.repo.candidates(ctx, o.OrganizationID, o.Since, candidatesPerRun)
	if err != nil {
		return err
	}
	// One campaign's connections are read once per run.
	byCampaign := map[string]*exports{}
	for _, cand := range cands {
		u, err := s.consider(ctx, o.OrganizationID, cand, byCampaign)
		if err != nil {
			// A lookup that failed (ClickHouse, Postgres) is tried again next
			// run: nothing is recorded for it.
			s.logger.Warn("offline conversions: looking at a conversion", "postback_id", cand.PostbackID, "error", err)
			continue
		}
		if err := s.repo.insert(ctx, u); err != nil {
			return err
		}
	}
	if _, err := s.repo.restate(ctx, o.OrganizationID); err != nil {
		return err
	}
	return nil
}

// exports are a campaign's source's connections; nil for none.
type exports struct {
	google  *connection
	metrica *metricaConnection
}

func (s *Service) campaignExports(ctx context.Context, orgID, campaignID string) (*exports, error) {
	var e exports
	c, found, err := s.repo.campaignConnection(ctx, orgID, campaignID)
	if err != nil {
		return nil, err
	}
	if found {
		e.google = &c
	}
	m, found, err := s.repo.campaignMetrica(ctx, orgID, campaignID)
	if err != nil {
		return nil, err
	}
	if found {
		e.metrica = &m
	}
	return &e, nil
}

func exportOn(enabled bool, since *time.Time, cand candidate) bool {
	return enabled && since != nil && !cand.CreatedAt.Before(*since)
}

// consider decides what becomes of one conversion: pending for Google or
// for Metrica (the network the click's own id is from), or skipped and
// why.
func (s *Service) consider(ctx context.Context, orgID string, cand candidate, byCampaign map[string]*exports) (upload, error) {
	u := upload{
		ID: s.newID(), OrganizationID: orgID, PostbackID: cand.PostbackID, ClickID: cand.ClickID,
		EventAt: cand.EventAt, Value: cand.Value, Currency: cand.Currency, Status: StatusSkipped,
	}
	if cand.ClickID == "" {
		u.SkipReason = SkipNotAttributed
		return u, nil
	}
	click, err := s.clicks.ByClickID(ctx, orgID, cand.ClickID)
	if errors.Is(err, attribution.ErrClickNotFound) {
		u.SkipReason = SkipClickNotFound
		return u, nil
	}
	if err != nil {
		return upload{}, err
	}
	exp, cached := byCampaign[click.CampaignID]
	if !cached {
		if exp, err = s.campaignExports(ctx, orgID, click.CampaignID); err != nil {
			return upload{}, err
		}
		byCampaign[click.CampaignID] = exp
	}
	platform := adclick.Platform(click.AdClickParam)
	g, m := exp.google, exp.metrica
	switch {
	case platform == adclick.Google && g != nil:
		// below
	case platform == adclick.Yandex && m != nil:
		return s.considerMetrica(ctx, u, *m, cand, click.OccurredAt, click.AdClickParam, click.AdClickID)
	case g != nil && exportOn(g.Enabled, g.Since, cand):
		u.ConnectionID, u.SkipReason = &g.ID, SkipNoGoogleClick
		return u, nil
	case m != nil && exportOn(m.Enabled, m.Since, cand):
		u.MetricaConnectionID, u.SkipReason = &m.ID, SkipNoYandexClick
		return u, nil
	case g != nil:
		u.ConnectionID, u.SkipReason = &g.ID, SkipNotConnected
		return u, nil
	case m != nil:
		u.MetricaConnectionID, u.SkipReason = &m.ID, SkipNotConnected
		return u, nil
	default:
		u.SkipReason = SkipNotConnected
		return u, nil
	}
	conn := g
	u.ConnectionID = &conn.ID
	if !exportOn(conn.Enabled, conn.Since, cand) {
		u.SkipReason = SkipNotConnected
		return u, nil
	}
	u.AdClickParam, u.AdClickID = click.AdClickParam, click.AdClickID
	if u.ActionID = actionFor(*conn, cand.Status); u.ActionID == "" {
		u.SkipReason = SkipNoAction
		return u, nil
	}
	u.Status = StatusPending
	return u, nil
}

// Send sends what is due. Errors of one source do not stop the others.
func (s *Service) Send(ctx context.Context) error {
	due, err := s.repo.claimDue(ctx, claimPerRun)
	if err != nil {
		return err
	}
	// Google's requests are per conversion action; a Metrica file holds
	// every goal of its counter.
	type key struct {
		org, connection, action string
		metrica                 bool
	}
	groups := map[key][]upload{}
	var order []key
	for _, u := range due {
		var k key
		switch {
		case u.ConnectionID != nil:
			k = key{org: u.OrganizationID, connection: *u.ConnectionID, action: u.ActionID}
		case u.MetricaConnectionID != nil:
			k = key{org: u.OrganizationID, connection: *u.MetricaConnectionID, metrica: true}
		default:
			continue
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], u)
	}
	var errs []error
	for _, k := range order {
		send := func() error { return s.sendGroup(ctx, k.org, k.connection, k.action, groups[k]) }
		if k.metrica {
			send = func() error { return s.sendMetricaGroup(ctx, k.org, k.connection, groups[k]) }
		}
		if err := send(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) sendGroup(ctx context.Context, orgID, connectionID, actionID string, uploads []upload) error {
	conn, err := s.repo.connectionByID(ctx, orgID, connectionID)
	if err != nil {
		return err
	}
	creds, err := s.creds.CredentialsByTrafficSourceID(ctx, orgID, conn.TrafficSourceID)
	if err != nil {
		return s.retryAll(ctx, uploads, "reading the connection's credentials: "+err.Error())
	}
	dest := googleads.Destination{AccountID: conn.AdAccountID, LoginAccountID: conn.LoginAccountID, ActionID: actionID}
	for start := 0; start < len(uploads); start += googleads.MaxEventsPerRequest {
		batch := uploads[start:min(start+googleads.MaxEventsPerRequest, len(uploads))]
		if err := s.sendBatch(ctx, creds, dest, batch); err != nil {
			return err
		}
	}
	return nil
}

func toEvent(u upload) googleads.Event {
	e := googleads.Event{At: u.EventAt, TransactionID: u.PostbackID, Value: u.Value}
	if u.Value != nil {
		e.Currency = u.Currency
	}
	switch u.AdClickParam {
	case adclick.GCLID:
		e.GCLID = u.AdClickID
	case adclick.GBRAID:
		e.GBRAID = u.AdClickID
	case adclick.WBRAID:
		e.WBRAID = u.AdClickID
	}
	return e
}

// sendBatch sends batch in one request; a refused request of several
// events is sent again one event at a time, so only the bad ones fail.
func (s *Service) sendBatch(ctx context.Context, creds adaccount.Credentials, dest googleads.Destination, batch []upload) error {
	events := make([]googleads.Event, len(batch))
	for i, u := range batch {
		events[i] = toEvent(u)
	}
	res, err := s.ingest.Ingest(ctx, creds, dest, events, false)
	switch {
	case err == nil:
		warning := strings.Join(res.Warnings, "; ")
		for _, u := range batch {
			if err := s.repo.markSent(ctx, u.ID, u.Value, res.RequestID, warning, ""); err != nil {
				return err
			}
			metrics.OfflineConversionsTotal.WithLabelValues("sent").Inc()
		}
		return nil
	case googleads.IsPermanent(err) && len(batch) > 1:
		for _, u := range batch {
			if err := s.sendBatch(ctx, creds, dest, []upload{u}); err != nil {
				return err
			}
		}
		return nil
	case googleads.IsPermanent(err):
		metrics.OfflineConversionsTotal.WithLabelValues("failed").Inc()
		return s.repo.markFailed(ctx, batch[0].ID, err.Error())
	default:
		return s.retryAll(ctx, batch, err.Error())
	}
}

func (s *Service) retryAll(ctx context.Context, uploads []upload, message string) error {
	for _, u := range uploads {
		failed, err := s.repo.markRetry(ctx, u.ID, message, retryDelay(u.Attempts))
		if err != nil {
			return err
		}
		if failed {
			metrics.OfflineConversionsTotal.WithLabelValues("failed").Inc()
		} else {
			metrics.OfflineConversionsTotal.WithLabelValues("retry").Inc()
		}
	}
	return nil
}

// RunLoop runs Enqueue, Send, then Check (Metrica's matching) every
// interval until ctx ends.
func (s *Service) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := s.Enqueue(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("offline conversions: enqueue", "error", err)
		}
		if err := s.Send(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("offline conversions: send", "error", err)
		}
		if s.metrica != nil {
			if err := s.Check(ctx); err != nil && ctx.Err() == nil {
				s.logger.Error("offline conversions: checking Metrica uploads", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
