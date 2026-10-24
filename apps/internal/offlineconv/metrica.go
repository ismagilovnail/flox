package offlineconv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/yandexmetrica"
)

// Offline conversions to Yandex Metrica (audit UNI-2 part 3). A source's
// Metrica connection names the counter on the landing, an OAuth token that
// can upload to it, the counter's currency and a goal per status. Clicks
// with yclid are reported to it by the worker, one file per counter and
// run; Check reads back how Metrica matched each file.

// Metrica is Yandex Metrica's Management API (yandexmetrica.Client).
type Metrica interface {
	Counter(ctx context.Context, token, counterID string) error
	Upload(ctx context.Context, token, counterID string, conversions []yandexmetrica.Conversion) (yandexmetrica.Uploading, error)
	Check(ctx context.Context, token, counterID string, uploadingID int64) (yandexmetrica.Uploading, error)
}

// WithMetrica sets the Metrica client: the API checks a counter with it
// when the settings are saved, the worker uploads.
func (s *Service) WithMetrica(m Metrica) *Service {
	s.metrica = m
	return s
}

const (
	// metricaRecheck: how long after a look a file not yet matched is looked
	// at again.
	metricaRecheck = 30 * time.Minute
	checksPerRun   = 200
)

var (
	counterIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
	currencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
)

// validGoal: a goal identifier as Metrica's JavaScript-event goals take
// it — no spaces or control characters, at most 255 characters.
func validGoal(g string) bool {
	if len(g) > 255 {
		return false
	}
	for _, r := range g {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == ',' || r == '"' {
			return false
		}
	}
	return true
}

func (s *Service) GetMetrica(ctx context.Context, orgID, trafficSourceID string) (MetricaStatus, error) {
	ok, err := s.repo.sourceExists(ctx, orgID, trafficSourceID)
	if err != nil {
		return MetricaStatus{}, err
	}
	if !ok {
		return MetricaStatus{}, apierror.NotFound("traffic source not found")
	}
	m, found, err := s.repo.metricaBySource(ctx, orgID, trafficSourceID)
	if err != nil {
		return MetricaStatus{}, err
	}
	if !found {
		return MetricaStatus{MetricaSettings: MetricaSettings{Currency: "RUB"}, Counts: map[string]int{}, Skipped: map[string]int{}}, nil
	}
	t, err := s.repo.tallyUploads(ctx, orgID, "metrica_connection_id", m.ID)
	if err != nil {
		return MetricaStatus{}, err
	}
	return MetricaStatus{
		Connected: true, TokenPreview: m.TokenPreview, MetricaSettings: m.MetricaSettings,
		Counts: t.Counts, Skipped: t.Skipped, Processing: t.Processing,
		LastError: t.LastError, LastErrorAt: t.LastErrorAt, LastSentAt: t.LastSentAt,
	}, nil
}

// PutMetrica validates and writes the connection. A new token or counter
// is checked with Metrica first, so a wrong one is refused at once.
func (s *Service) PutMetrica(ctx context.Context, orgID, trafficSourceID string, in MetricaInput) (MetricaStatus, error) {
	ok, err := s.repo.sourceExists(ctx, orgID, trafficSourceID)
	if err != nil {
		return MetricaStatus{}, err
	}
	if !ok {
		return MetricaStatus{}, apierror.NotFound("traffic source not found")
	}
	existing, found, err := s.repo.metricaBySource(ctx, orgID, trafficSourceID)
	if err != nil {
		return MetricaStatus{}, err
	}
	in.Token = strings.TrimSpace(in.Token)
	in.CounterID = strings.TrimSpace(in.CounterID)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.LeadGoal = strings.TrimSpace(in.LeadGoal)
	in.PurchaseGoal = strings.TrimSpace(in.PurchaseGoal)
	in.RepurchaseGoal = strings.TrimSpace(in.RepurchaseGoal)
	if in.Currency == "" {
		in.Currency = "RUB"
	}

	fields := map[string]string{}
	if !counterIDPattern.MatchString(in.CounterID) {
		fields["counterId"] = "the Metrica counter's number: digits only"
	}
	if in.Token == "" && !found {
		fields["token"] = "an OAuth token is needed to connect the counter"
	}
	if strings.ContainsFunc(in.Token, unicode.IsSpace) || len(in.Token) > 512 {
		fields["token"] = "an OAuth token: no spaces"
	}
	if !currencyPattern.MatchString(in.Currency) {
		fields["currency"] = "the counter's currency: a 3-letter ISO 4217 code"
	}
	for name, v := range map[string]string{"leadGoal": in.LeadGoal, "purchaseGoal": in.PurchaseGoal, "repurchaseGoal": in.RepurchaseGoal} {
		if !validGoal(v) {
			fields[name] = "a goal identifier: no spaces, commas or quotes, at most 255 characters"
		}
	}
	if in.Enabled && in.LeadGoal == "" && in.PurchaseGoal == "" && in.RepurchaseGoal == "" {
		fields["purchaseGoal"] = "name at least one goal to turn the export on"
	}
	if len(fields) > 0 {
		return MetricaStatus{}, apierror.Validation("invalid Yandex Metrica settings", fields)
	}

	if in.Token != "" || !found || in.CounterID != existing.CounterID {
		token := in.Token
		if token == "" {
			if token, err = s.repo.token(existing); err != nil {
				return MetricaStatus{}, err
			}
		}
		if err := s.checkCounter(ctx, token, in.CounterID); err != nil {
			return MetricaStatus{}, err
		}
	}
	if err := s.repo.putMetrica(ctx, orgID, trafficSourceID, s.newID(), in.Token, in.MetricaSettings); err != nil {
		return MetricaStatus{}, err
	}
	return s.GetMetrica(ctx, orgID, trafficSourceID)
}

// checkCounter turns Metrica's answer about the counter into a field
// error the form can show.
func (s *Service) checkCounter(ctx context.Context, token, counterID string) error {
	if s.metrica == nil {
		return errors.New("offlineconv: no Metrica client")
	}
	err := s.metrica.Counter(ctx, token, counterID)
	var me *yandexmetrica.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &me) && me.Status == http.StatusNotFound:
		return apierror.Validation("Metrica has no such counter", map[string]string{"counterId": "Metrica has no counter with this number"})
	case yandexmetrica.IsAuth(err):
		return apierror.Validation("Metrica refused the token", map[string]string{
			"token": "Metrica refused the token, or it has no access to this counter: " + me.Message})
	default:
		s.logger.Warn("offline conversions: checking a Metrica counter", "error", err)
		return apierror.Unavailable("Yandex Metrica did not answer; try again")
	}
}

func (s *Service) DeleteMetrica(ctx context.Context, orgID, trafficSourceID string) error {
	ok, err := s.repo.deleteMetrica(ctx, orgID, trafficSourceID)
	if err != nil {
		return err
	}
	if !ok {
		return apierror.NotFound("no Yandex Metrica counter connected for this traffic source")
	}
	return nil
}

// --- the worker's ---

// goalFor is the goal a status is reported as; empty for none.
func goalFor(m metricaConnection, status string) string {
	switch event.Type(status) {
	case event.CpaHold:
		return m.LeadGoal
	case event.CpaAccept:
		return m.PurchaseGoal
	case event.CpaRedep:
		return m.RepurchaseGoal
	}
	return ""
}

// considerMetrica fills u for Metrica: pending with the goal and the value
// in the counter's currency, or skipped and why.
func (s *Service) considerMetrica(ctx context.Context, u upload, m metricaConnection, cand candidate, clickAt time.Time, adClickParam, adClickID string) (upload, error) {
	u.MetricaConnectionID = &m.ID
	if !m.Enabled || m.Since == nil || cand.CreatedAt.Before(*m.Since) {
		u.SkipReason = SkipNotConnected
		return u, nil
	}
	if !clickAt.IsZero() && s.now().Sub(clickAt) > yandexmetrica.VisitWindow {
		u.SkipReason = SkipVisitTooOld
		return u, nil
	}
	u.AdClickParam, u.AdClickID = adClickParam, adClickID
	if u.ActionID = goalFor(m, cand.Status); u.ActionID == "" {
		u.SkipReason = SkipNoAction
		return u, nil
	}
	// Metrica zeroes a price in another currency than the counter's: the
	// value goes converted at the event's date, or without a price when
	// there is no rate (never as 0).
	if u.Value != nil && u.Currency != m.Currency {
		v, err := s.repo.inCurrency(ctx, *u.Value, u.Currency, m.Currency, u.EventAt)
		if err != nil {
			return upload{}, err
		}
		u.Value = v
	}
	u.Currency = m.Currency
	u.Status = StatusPending
	return u, nil
}

func (s *Service) sendMetricaGroup(ctx context.Context, orgID, connectionID string, uploads []upload) error {
	m, err := s.repo.metricaByID(ctx, orgID, connectionID)
	if err != nil {
		return err
	}
	token, err := s.repo.token(m)
	if err != nil {
		return s.retryAll(ctx, uploads, "reading the Metrica token: "+err.Error())
	}
	for start := 0; start < len(uploads); start += yandexmetrica.MaxConversionsPerUpload {
		batch := uploads[start:min(start+yandexmetrica.MaxConversionsPerUpload, len(uploads))]
		if err := s.sendMetricaBatch(ctx, token, m.CounterID, batch); err != nil {
			return err
		}
	}
	return nil
}

// sendMetricaBatch uploads batch as one file. A refused file of several
// rows is split in halves (not row by row: Metrica allows 5000 requests a
// day per user), so only the bad rows fail.
func (s *Service) sendMetricaBatch(ctx context.Context, token, counterID string, batch []upload) error {
	rows := make([]yandexmetrica.Conversion, len(batch))
	for i, u := range batch {
		rows[i] = yandexmetrica.Conversion{Yclid: u.AdClickID, Goal: u.ActionID, At: u.EventAt, Price: u.Value, Currency: u.Currency}
	}
	res, err := s.metrica.Upload(ctx, token, counterID, rows)
	switch {
	case err == nil:
		warning := ""
		if res.LineQuantity < res.SourceQuantity {
			warning = fmt.Sprintf("Metrica took %d of the file's %d rows", res.LineQuantity, res.SourceQuantity)
		}
		id := strconv.FormatInt(res.ID, 10)
		for _, u := range batch {
			if err := s.repo.markSent(ctx, u.ID, u.Value, id, warning, res.Status); err != nil {
				return err
			}
			metrics.OfflineConversionsTotal.WithLabelValues("sent").Inc()
		}
		return nil
	case yandexmetrica.IsPermanent(err) && len(batch) > 1:
		half := len(batch) / 2
		if err := s.sendMetricaBatch(ctx, token, counterID, batch[:half]); err != nil {
			return err
		}
		return s.sendMetricaBatch(ctx, token, counterID, batch[half:])
	case yandexmetrica.IsPermanent(err):
		metrics.OfflineConversionsTotal.WithLabelValues("failed").Inc()
		return s.repo.markFailed(ctx, batch[0].ID, err.Error())
	default:
		return s.retryAll(ctx, batch, err.Error())
	}
}

// linkageFailure is what a file Metrica matched no visit of fails with.
const linkageFailure = "Yandex Metrica matched none of the file's yclids to a visit (LINKAGE_FAILURE): " +
	"the counter must be on the landing, the yclid in the landing's address ({yclid} in the offer URL), " +
	"and the goals must exist in the counter"

// Check reads back how Metrica matched the files sent lately.
func (s *Service) Check(ctx context.Context) error {
	files, err := s.repo.uncheckedUploads(ctx, metricaRecheck, checksPerRun)
	if err != nil {
		return err
	}
	tokens := map[string]metricaToken{}
	var errs []error
	for _, f := range files {
		tok, ok := tokens[f.MetricaConnectionID]
		if !ok {
			tok = s.metricaToken(ctx, f.OrganizationID, f.MetricaConnectionID)
			tokens[f.MetricaConnectionID] = tok
		}
		id, perr := strconv.ParseInt(f.RequestID, 10, 64)
		var res yandexmetrica.Uploading
		err := tok.err
		if err == nil {
			err = perr
		}
		if err == nil {
			res, err = s.metrica.Check(ctx, tok.token, tok.counterID, id)
		}
		if err != nil {
			s.logger.Warn("offline conversions: checking a Metrica upload", "upload", f.RequestID, "error", err)
			if err := s.repo.touchUnchecked(ctx, f); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err := s.repo.setRemoteStatus(ctx, f, res.Status, linkageFailure); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type metricaToken struct {
	token, counterID string
	err              error
}

func (s *Service) metricaToken(ctx context.Context, orgID, connectionID string) metricaToken {
	m, err := s.repo.metricaByID(ctx, orgID, connectionID)
	if err != nil {
		return metricaToken{err: err}
	}
	token, err := s.repo.token(m)
	return metricaToken{token: token, counterID: m.CounterID, err: err}
}
