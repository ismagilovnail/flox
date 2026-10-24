package shopify

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
)

// Platform is shop_connections.platform for a Shopify store.
const Platform = "shopify"

// ErrNotConnected: the conversion source has no Shopify store.
var ErrNotConnected = errors.New("shopify: no store connected")

// ErrNoKey: SHOP_WEBHOOK_ENCRYPTION_KEY is not set, so a signing secret can
// be neither stored nor checked.
var ErrNoKey = errors.New("shopify: SHOP_WEBHOOK_ENCRYPTION_KEY is not set")

// Store reads and writes shop_connections and looks up the orders a
// store's webhooks are about. Settings methods are scoped to the session's
// organization; Connection (the tracker's) is keyed by the network id in
// the webhook URL, like conversion.NetworkLookup, and returns the
// organization it belongs to.
type Store struct {
	db  *pgxpool.Pool
	key []byte // nil: ErrNoKey
}

func NewStore(db *pgxpool.Pool, key []byte) *Store { return &Store{db: db, key: key} }

// Settings is what the conversion source's page shows: never the secret,
// only its last 4 characters.
type Settings struct {
	Connected     bool   `json:"connected"`
	ShopDomain    string `json:"shopDomain"`
	SecretPreview string `json:"secretPreview"`
}

// networkTemplate is the network's template key; found false when it is
// not orgID's.
func (s *Store) networkTemplate(ctx context.Context, orgID, networkID string) (string, bool, error) {
	var key string
	err := s.db.QueryRow(ctx, `SELECT template_key FROM networks WHERE organization_id = $1 AND id = $2`, orgID, networkID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("shopify: reading the conversion source: %w", err)
	}
	return key, true, nil
}

func (s *Store) settings(ctx context.Context, orgID, networkID string) (Settings, error) {
	var st Settings
	err := s.db.QueryRow(ctx, `
		SELECT shop_domain, secret_preview FROM shop_connections
		WHERE organization_id = $1 AND network_id = $2 AND platform = $3`, orgID, networkID, Platform).Scan(&st.ShopDomain, &st.SecretPreview)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("shopify: reading the store connection: %w", err)
	}
	st.Connected = true
	return st, nil
}

// put creates or replaces the connection; secret "" keeps the stored one
// (the caller has checked there is one: the insert's empty placeholder
// never lands).
func (s *Store) put(ctx context.Context, id, orgID, networkID, shopDomain, secret string) error {
	var cipher, preview *string
	if secret != "" {
		if s.key == nil {
			return ErrNoKey
		}
		enc, err := adaccount.Seal(s.key, secret)
		if err != nil {
			return err
		}
		p := secret[max(len(secret)-4, 0):]
		cipher, preview = &enc, &p
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO shop_connections (id, organization_id, network_id, platform, shop_domain, secret, secret_preview)
		VALUES ($1, $2, $3, $4, $5, coalesce($6, ''), coalesce($7, ''))
		ON CONFLICT (network_id, platform) DO UPDATE SET
			shop_domain = EXCLUDED.shop_domain,
			secret = coalesce($6, shop_connections.secret),
			secret_preview = coalesce($7, shop_connections.secret_preview)
		WHERE shop_connections.organization_id = $2`,
		id, orgID, networkID, Platform, shopDomain, cipher, preview)
	if err != nil {
		return fmt.Errorf("shopify: writing the store connection: %w", err)
	}
	return nil
}

func (s *Store) remove(ctx context.Context, orgID, networkID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM shop_connections WHERE organization_id = $1 AND network_id = $2 AND platform = $3`,
		orgID, networkID, Platform)
	if err != nil {
		return false, fmt.Errorf("shopify: removing the store connection: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// --- the tracker's ---

// Connection is a network's store, its secret decrypted, for checking a
// webhook's signature.
type Connection struct {
	OrganizationID string
	ShopDomain     string
	Secret         string
}

// Connection is networkID's Shopify store; ErrNotConnected when it has
// none.
func (s *Store) Connection(ctx context.Context, networkID string) (Connection, error) {
	var c Connection
	var cipher string
	err := s.db.QueryRow(ctx, `
		SELECT organization_id, shop_domain, secret FROM shop_connections WHERE network_id = $1 AND platform = $2`,
		networkID, Platform).Scan(&c.OrganizationID, &c.ShopDomain, &cipher)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotConnected
	}
	if err != nil {
		return Connection{}, fmt.Errorf("shopify: reading the store connection: %w", err)
	}
	if s.key == nil {
		return Connection{}, ErrNoKey
	}
	if c.Secret, err = adaccount.Open(s.key, cipher); err != nil {
		return Connection{}, err
	}
	return c, nil
}

// OrderRecorded: the order is already a recorded purchase or repeat
// purchase of the source — a replayed webhook.
func (s *Store) OrderRecorded(ctx context.Context, orgID, networkID, orderID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM postbacks
			WHERE organization_id = $1 AND network_id = $2 AND network_txn_id = $3
			  AND result = 'success' AND direction = 'incoming' AND status IN ('CPA_ACCEPT', 'CPA_REDEP'))`,
		orgID, networkID, orderID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("shopify: looking up the order: %w", err)
	}
	return ok, nil
}

// ClickBought: the click already has a recorded purchase, so another order
// is a repeat purchase.
func (s *Store) ClickBought(ctx context.Context, orgID, clickID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM postbacks
			WHERE organization_id = $1 AND click_id = $2 AND result = 'success' AND direction = 'incoming' AND status = 'CPA_ACCEPT')`,
		orgID, clickID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("shopify: looking up the click's purchases: %w", err)
	}
	return ok, nil
}

// OrderClick is the click of the recorded purchase an order id names;
// found false when the order is not one FLOX recorded (bought without a
// click it knows).
func (s *Store) OrderClick(ctx context.Context, orgID, networkID, orderID string) (clickID string, found bool, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT click_id FROM postbacks
		WHERE organization_id = $1 AND network_id = $2 AND network_txn_id = $3
		  AND result = 'success' AND direction = 'incoming' AND status IN ('CPA_ACCEPT', 'CPA_REDEP')
		ORDER BY created_at LIMIT 1`, orgID, networkID, orderID).Scan(&clickID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("shopify: looking up the refunded order: %w", err)
	}
	return clickID, true, nil
}
