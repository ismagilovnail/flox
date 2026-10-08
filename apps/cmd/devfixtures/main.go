// Command devfixtures fills an empty dev database with ready-to-use accounts
// and a minimal working setup (`make fixtures`, usually right after
// `make dev-reset`).
//
// Everything goes through the real service layer — Signup + the emailed
// verification link for owners, Invite + AcceptInvite for the other roles,
// the domain/network/offer/campaign/stream-set services for the demo data —
// so fixtures obey the same validation, hashing and tenant scoping as the
// app itself and never drift from it.
//
// Accounts (password FIXTURES_PASSWORD, default below):
//
//	FLOX Demo  — owner, admin, manager, buyer, analyst, viewer @flox.local
//	FLOX Other — other-owner@flox.local (a second tenant, for isolation checks)
//
// Demo data in FLOX Demo: tracking domain track.localhost, a traffic source,
// a network with status mappings, an offer, an active campaign whose stream
// set routes everything to the offer, and the tracking link /t/demo. The
// network's postback secret is shown once, here — it is stored hashed.
//
// Dev only: it refuses to run against an organization name that already
// exists, so it never mixes into data it did not create.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/domain"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventmapping"
	"github.com/ismagilovnail/flox/apps/internal/network"
	"github.com/ismagilovnail/flox/apps/internal/offer"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/trackinglink"
	"github.com/ismagilovnail/flox/apps/internal/trafficsource"
)

const defaultPassword = "flox-demo-2026"

const (
	demoOrg  = "FLOX Demo"
	otherOrg = "FLOX Other"

	trackingHost = "track.localhost"
	trackingSlug = "demo"
)

// inviteRoles are created in FLOX Demo next to its owner, one account each.
var inviteRoles = []struct{ email, name, role string }{
	{"admin@flox.local", "Demo Admin", "Admin"},
	{"manager@flox.local", "Demo Manager", "Manager"},
	{"buyer@flox.local", "Demo Buyer", "Buyer"},
	{"analyst@flox.local", "Demo Analyst", "Analyst"},
	{"viewer@flox.local", "Demo Viewer", "Viewer"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Фикстуры не созданы:", err)
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) && len(apiErr.Fields) > 0 {
			fmt.Fprintln(os.Stderr, "  fields:", apiErr.Fields)
		}
		os.Exit(1)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("не задан DATABASE_URL (make fixtures подставляет его сам)")
	}
	password := envOr("FIXTURES_PASSWORD", defaultPassword)
	appURL := envOr("APP_URL", "http://localhost:3000")
	trackerURL := envOr("FIXTURES_TRACKER_URL", "http://localhost:8081")

	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer db.Close()

	for _, name := range []string{demoOrg, otherOrg} {
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE name = $1)`, name).Scan(&exists); err != nil {
			return fmt.Errorf("checking for %q: %w", name, err)
		}
		if exists {
			return fmt.Errorf("организация %q уже есть — фикстуры создаются только в пустой базе. Сначала сброс: make dev-fresh CONFIRM=yes (сброс + фикстуры) или make dev-reset CONFIRM=yes", name)
		}
	}

	mail := &capturingMailer{}
	authSvc := auth.NewService(auth.NewRepository(db), appURL, mail)

	demoOrgID, ownerID, err := createOwner(ctx, authSvc, mail, demoOrg, "owner@flox.local", "Demo Owner", password)
	if err != nil {
		return err
	}
	for _, r := range inviteRoles {
		if err := addMember(ctx, authSvc, demoOrgID, ownerID, r.email, r.name, r.role, password); err != nil {
			return err
		}
	}
	if _, _, err := createOwner(ctx, authSvc, mail, otherOrg, "other-owner@flox.local", "Other Owner", password); err != nil {
		return err
	}

	demo, err := seedDemo(ctx, db, demoOrgID, trackerURLWithHost(trackerURL, trackingHost)+"/t/"+trackingSlug)
	if err != nil {
		return err
	}

	fmt.Printf(`
Фикстуры созданы.

Вход: %s — пароль у всех аккаунтов: %s

  FLOX Demo   owner@flox.local        Владелец
              admin@flox.local        Администратор
              manager@flox.local      Менеджер
              buyer@flox.local        Байер
              analyst@flox.local      Аналитик
              viewer@flox.local       Наблюдатель
  FLOX Other  other-owner@flox.local  Владелец (вторая организация, для проверки изоляции)

Трекинговая ссылка:  %s
  (хост %s указывает на 127.0.0.1; трекер узнаёт ссылку по хосту)

Постбэк сети «%s» (статусы reg / ftd / redep / rejected):
  %s/postback/%s?secret=%s&click_id={click_id}&status=ftd&revenue=25&currency=USD

Секрет постбэка хранится в виде хеша — показан только сейчас, сохраните его.
Исходящие постбэки уходят на partner.example (не существует): повторяются и
попадают в dead-letter. Чтобы проверить доставку, укажите в сети настоящий URL.
`, appURL, password,
		trackerURLWithHost(trackerURL, trackingHost)+"/t/"+trackingSlug, trackingHost,
		demo.networkName, trackerURL, demo.networkID, demo.postbackSecret)
	return nil
}

// createOwner signs up an organization and verifies its owner through the
// emailed link, exactly as a real signup does.
func createOwner(ctx context.Context, svc *auth.Service, mail *capturingMailer, orgName, email, name, password string) (orgID, userID string, err error) {
	res, err := svc.Signup(ctx, auth.SignupInput{OrganizationName: orgName, Name: name, Email: email, Password: password})
	if err != nil {
		return "", "", fmt.Errorf("signing up %s: %w", email, err)
	}
	token, err := mail.tokenFor(email)
	if err != nil {
		return "", "", err
	}
	if _, err := svc.VerifyEmail(ctx, token, "127.0.0.1"); err != nil {
		return "", "", fmt.Errorf("verifying %s: %w", email, err)
	}
	return res.Organization.ID, res.User.ID, nil
}

// addMember invites email with role and accepts the invite with password.
func addMember(ctx context.Context, svc *auth.Service, orgID, ownerID, email, name, role, password string) error {
	inviteURL, err := svc.Invite(ctx, orgID, ownerID, auth.InviteInput{Email: email, Name: name, RoleKey: role})
	if err != nil {
		return fmt.Errorf("inviting %s as %s: %w", email, role, err)
	}
	token, err := tokenFrom(inviteURL)
	if err != nil {
		return fmt.Errorf("invite link for %s: %w", email, err)
	}
	if _, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: token, Name: name, Password: password, ClientIP: "127.0.0.1"}); err != nil {
		return fmt.Errorf("accepting invite for %s: %w", email, err)
	}
	return nil
}

type demoSetup struct {
	networkID, networkName, postbackSecret string
}

func seedDemo(ctx context.Context, db *pgxpool.Pool, orgID, trackingURL string) (demoSetup, error) {
	var out demoSetup

	// Only Create is used, which needs no registrar/DNS adapters.
	dom, err := domain.NewService(domain.NewRepository(db), nil, nil, nil, "", nil).Create(ctx, orgID, domain.CreateInput{
		Domain: trackingHost, Purpose: []string{"tracking"},
	})
	if err != nil {
		return out, fmt.Errorf("creating domain: %w", err)
	}

	source, err := trafficsource.NewService(trafficsource.NewRepository(db)).Create(ctx, orgID, trafficsource.CreateInput{
		Name:             "Facebook Demo",
		Type:             "Facebook",
		TrackingTemplate: trackingURL,
		CostIntegration:  trafficsource.CostIntegrationNone,
	})
	if err != nil {
		return out, fmt.Errorf("creating traffic source: %w", err)
	}

	net, err := network.NewService(network.NewRepository(db)).Create(ctx, orgID, network.CreateInput{
		Name: "Demo Network",
		// RFC 2606 reserved: never resolves, so demo conversions are never
		// forwarded anywhere real (those deliveries retry, then dead-letter).
		PostbackURL: "https://partner.example/postback?click_id={click_id}&status={status}&payout={revenue}",
	})
	if err != nil {
		return out, fmt.Errorf("creating network: %w", err)
	}
	out.networkID, out.networkName, out.postbackSecret = net.Network.ID, net.Network.Name, net.PostbackSecret

	mappings := eventmapping.NewService(eventmapping.NewRepository(db))
	for raw, status := range map[string]event.Type{
		"reg": event.CpaHold, "ftd": event.CpaAccept, "redep": event.CpaRedep, "rejected": event.CpaDecline,
	} {
		if _, err := mappings.Create(ctx, orgID, eventmapping.CreateInput{NetworkID: net.Network.ID, NetworkStatus: raw, FloxStatus: status}); err != nil {
			return out, fmt.Errorf("creating mapping %s: %w", raw, err)
		}
	}

	off, err := offer.NewService(offer.NewRepository(db)).Create(ctx, orgID, offer.CreateInput{
		NetworkID: net.Network.ID,
		Name:      "Demo Offer",
		Countries: []string{"US", "DE", "GB"},
		Payout:    25,
		Currency:  "USD",
		Links:     []offer.LinkInput{{Label: "Main", URL: "https://offer.example.com/?click_id={click_id}"}},
	})
	if err != nil {
		return out, fmt.Errorf("creating offer: %w", err)
	}

	campSvc := campaign.NewService(campaign.NewRepository(db))
	camp, err := campSvc.Create(ctx, orgID, campaign.CreateInput{
		TrafficSourceID: source.ID,
		Name:            "Demo Campaign",
		FallbackURL:     "https://example.com/fallback",
	})
	if err != nil {
		return out, fmt.Errorf("creating campaign: %w", err)
	}

	if _, err := streamset.NewService(streamset.NewRepository(db)).Create(ctx, orgID, camp.ID, streamset.CreateInput{
		Name:        "All traffic",
		Priority:    1,
		FallbackURL: "https://example.com/fallback",
		RootFilter: streamset.FilterNode{
			Kind:   streamset.NodeGroup,
			Joiner: routing.JoinAND,
			Children: []streamset.FilterNode{
				{Kind: streamset.NodeCondition, Field: routing.FieldBot, Operator: routing.OpIs, Value: "0"},
			},
		},
		Flows: []streamset.FlowInput{{
			Name: "Demo Offer", Active: true, Weight: 100,
			Destination: streamset.Destination{Kind: routing.DestinationOffer, NetworkID: net.Network.ID, OfferID: off.ID},
		}},
	}); err != nil {
		return out, fmt.Errorf("creating stream set: %w", err)
	}

	if _, err := campSvc.Activate(ctx, orgID, camp.ID); err != nil {
		return out, fmt.Errorf("activating campaign: %w", err)
	}

	if _, err := trackinglink.NewService(trackinglink.NewRepository(db)).Create(ctx, orgID, camp.ID, trackinglink.CreateInput{
		DomainID: dom.ID, Slug: trackingSlug,
	}); err != nil {
		return out, fmt.Errorf("creating tracking link: %w", err)
	}
	return out, nil
}

// capturingMailer stands in for SMTP: it keeps the verification links so
// owners are verified through the real VerifyEmail path.
type capturingMailer struct {
	mu   sync.Mutex
	sent map[string]string // recipient -> body
}

func (m *capturingMailer) Send(_ context.Context, to, _, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sent == nil {
		m.sent = map[string]string{}
	}
	m.sent[to] = body
	return nil
}

func (m *capturingMailer) tokenFor(email string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.sent[email]
	if !ok {
		return "", fmt.Errorf("no verification email was sent to %s", email)
	}
	return tokenFrom(body)
}

var tokenPattern = regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`)

func tokenFrom(s string) (string, error) {
	m := tokenPattern.FindStringSubmatch(s)
	if m == nil {
		return "", errors.New("no token in link")
	}
	return m[1], nil
}

// trackerURLWithHost swaps the tracker URL's host for the tracking domain,
// keeping scheme and port: http://localhost:8081 -> http://track.localhost:8081.
func trackerURLWithHost(trackerURL, host string) string {
	re := regexp.MustCompile(`^(https?://)[^:/]+`)
	return re.ReplaceAllString(trackerURL, "${1}"+host)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
