package adaccount

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db  *pgxpool.Pool
	key []byte
}

// key encrypts/decrypts every secret column on ad_account_connections
// (see crypto.go): Google Ads' own developer_token/client_secret/
// refresh_token, and facebook_ads/tiktok_ads' own access_token.
func NewRepository(db *pgxpool.Pool, key []byte) *Repository { return &Repository{db: db, key: key} }

// selectColumns never includes the plaintext of access_token or any of
// Google Ads' own secret columns — every public read goes through this,
// so there is no query path in this package that could accidentally
// leak a raw credential into a JSON response. Both access_token and
// refresh_token come back here still ciphertext; scanConnection decrypts
// whichever one is present (via the repository's own key) purely to
// compute its own last-4 preview, never returning the decrypted value
// itself — sliced ciphertext would be meaningless.
const selectColumns = `id, organization_id, traffic_source_id, ad_account_id, access_token, refresh_token, timezone, created_at, updated_at`

func (r *Repository) scanConnection(row pgx.Row) (Connection, error) {
	var c Connection
	var encryptedAccessToken *string
	var encryptedRefreshToken *string
	if err := row.Scan(&c.ID, &c.OrganizationID, &c.TrafficSourceID, &c.AdAccountID, &encryptedAccessToken, &encryptedRefreshToken, &c.Timezone, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return Connection{}, err
	}
	encryptedSecret := encryptedAccessToken
	if encryptedSecret == nil {
		encryptedSecret = encryptedRefreshToken
	}
	if encryptedSecret != nil {
		secret, err := decrypt(r.key, *encryptedSecret)
		if err != nil {
			return Connection{}, err
		}
		if len(secret) > 4 {
			secret = secret[len(secret)-4:]
		}
		c.TokenPreview = secret
	}
	return c, nil
}

func (r *Repository) GetByTrafficSourceID(ctx context.Context, orgID, trafficSourceID string) (Connection, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM ad_account_connections
		WHERE traffic_source_id = $1 AND organization_id = $2`,
		trafficSourceID, orgID,
	)
	c, err := r.scanConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, apierror.NotFound("no ad account connected for this traffic source")
	}
	return c, err
}

// Connect upserts by traffic_source_id — re-connecting (a fresh
// credential, or the same one re-pasted) replaces the row in place, same
// "re-submitting updates it, it doesn't stack" precedent as
// cost_entries' own dedup-key-as-identity design (00009). Whichever of
// AccessToken/DeveloperToken+ClientID+ClientSecret+RefreshToken is empty
// (Service.Connect has already validated the right subset is non-empty
// for this connection's actual platform) is stored as SQL NULL, not an
// empty string — a facebook_ads/tiktok_ads row has real NULLs in the
// three Google Ads columns, and vice versa.
func (r *Repository) Connect(ctx context.Context, id, orgID, trafficSourceID string, in ConnectInput) (Connection, error) {
	accessToken, err := encryptOptional(r.key, in.AccessToken)
	if err != nil {
		return Connection{}, err
	}
	developerToken, err := encryptOptional(r.key, in.DeveloperToken)
	if err != nil {
		return Connection{}, err
	}
	clientSecret, err := encryptOptional(r.key, in.ClientSecret)
	if err != nil {
		return Connection{}, err
	}
	refreshToken, err := encryptOptional(r.key, in.RefreshToken)
	if err != nil {
		return Connection{}, err
	}
	var clientID *string
	if in.ClientID != "" {
		clientID = &in.ClientID
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO ad_account_connections
			(id, organization_id, traffic_source_id, ad_account_id, access_token, developer_token, client_id, client_secret, refresh_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (traffic_source_id) DO UPDATE SET
			ad_account_id = EXCLUDED.ad_account_id,
			access_token = EXCLUDED.access_token,
			developer_token = EXCLUDED.developer_token,
			client_id = EXCLUDED.client_id,
			client_secret = EXCLUDED.client_secret,
			refresh_token = EXCLUDED.refresh_token,
			-- Another account is another zone: learned again at its first sync.
			timezone = CASE WHEN ad_account_connections.ad_account_id = EXCLUDED.ad_account_id
			                THEN ad_account_connections.timezone ELSE '' END
		RETURNING `+selectColumns,
		id, orgID, trafficSourceID, in.AdAccountID, accessToken, developerToken, clientID, clientSecret, refreshToken,
	)
	return r.scanConnection(row)
}

func (r *Repository) Disconnect(ctx context.Context, orgID, trafficSourceID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM ad_account_connections WHERE traffic_source_id = $1 AND organization_id = $2`, trafficSourceID, orgID)
	if err != nil {
		return fmt.Errorf("disconnecting ad account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("no ad account connected for this traffic source")
	}
	return nil
}

// CredentialsByTrafficSourceID returns the real access token for a
// trusted, Go-only caller (the ad-spend sync, §74/§27-COST Phase B) —
// structurally separate from GetByTrafficSourceID's Connection return
// (no AccessToken field at all, see Connection's own doc comment above
// selectColumns) so there is no HTTP-reachable code path that could ever
// be edited into also exposing the raw token; only a caller holding a
// *Repository directly (never just the public Service) can reach this.
func (r *Repository) CredentialsByTrafficSourceID(ctx context.Context, orgID, trafficSourceID string) (Credentials, error) {
	var c Credentials
	var accessToken, clientID, developerToken, clientSecret, refreshToken *string
	err := r.db.QueryRow(ctx,
		`SELECT ad_account_id, access_token, client_id, developer_token, client_secret, refresh_token
		 FROM ad_account_connections WHERE traffic_source_id = $1 AND organization_id = $2`,
		trafficSourceID, orgID,
	).Scan(&c.AdAccountID, &accessToken, &clientID, &developerToken, &clientSecret, &refreshToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, apierror.NotFound("no ad account connected for this traffic source")
	}
	if err != nil {
		return Credentials{}, err
	}
	if clientID != nil {
		c.ClientID = *clientID
	}
	if c.AccessToken, err = decryptOptional(r.key, accessToken); err != nil {
		return Credentials{}, err
	}
	if c.DeveloperToken, err = decryptOptional(r.key, developerToken); err != nil {
		return Credentials{}, err
	}
	if c.ClientSecret, err = decryptOptional(r.key, clientSecret); err != nil {
		return Credentials{}, err
	}
	if c.RefreshToken, err = decryptOptional(r.key, refreshToken); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

// ListAllConnections returns every connected ad account across every org —
// deliberately unscoped by orgID, unlike every other method here, since its
// only caller is the ad-spend sync scheduler (§74/§27-COST) driven by a
// timer, not a tenant's request. Never HTTP-reachable: there is no handler
// anywhere in this package that calls it.
func (r *Repository) ListAllConnections(ctx context.Context) ([]ConnectionRef, error) {
	rows, err := r.db.Query(ctx, `SELECT organization_id, traffic_source_id FROM ad_account_connections ORDER BY organization_id, traffic_source_id`)
	if err != nil {
		return nil, fmt.Errorf("listing ad account connections: %w", err)
	}
	defer rows.Close()

	var refs []ConnectionRef
	for rows.Next() {
		var ref ConnectionRef
		if err := rows.Scan(&ref.OrganizationID, &ref.TrafficSourceID); err != nil {
			return nil, fmt.Errorf("scanning ad account connection: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// PlatformCount is one row of AdPlatformSummary's result — how many of an
// org's traffic sources configured for a given cost-integration platform
// actually have a real connected ad account.
type PlatformCount struct {
	CostIntegration string
	ConnectedCount  int
	TotalCount      int
}

// AdPlatformSummary powers Settings > Integrations' read-only ad-platform
// cards (apps/internal/integrations) — a real aggregate over this
// package's own data, not a second store of connection state. Grouped by
// traffic_sources.cost_integration (not joined against a separate
// provider table), so once a platform is in the WHERE clause below it
// needs no further change here — any traffic source that sets its
// cost_integration to that value is picked up automatically. The WHERE
// clause itself still needs a platform added by name when a new one
// launches (google_ads was added here in the same phase that built its
// own real CostProvider adapter).
func (r *Repository) AdPlatformSummary(ctx context.Context, orgID string) ([]PlatformCount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ts.cost_integration, count(aac.id), count(ts.id)
		FROM traffic_sources ts
		LEFT JOIN ad_account_connections aac ON aac.traffic_source_id = ts.id
		WHERE ts.organization_id = $1 AND ts.cost_integration IN ('facebook_ads', 'tiktok_ads', 'google_ads')
		GROUP BY ts.cost_integration
		ORDER BY ts.cost_integration`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("summarizing ad platform connections: %w", err)
	}
	defer rows.Close()

	var out []PlatformCount
	for rows.Next() {
		var p PlatformCount
		if err := rows.Scan(&p.CostIntegration, &p.ConnectedCount, &p.TotalCount); err != nil {
			return nil, fmt.Errorf("scanning ad platform summary: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TrafficSourceCostIntegration returns the traffic source's own
// cost_integration value (a raw query against trafficsource's table,
// same as streamset.Repository's own cross-domain checks — not an
// import of the trafficsource package, to avoid a needless dependency
// for one column read) and whether it belongs to orgID at all.
func (r *Repository) TrafficSourceCostIntegration(ctx context.Context, orgID, trafficSourceID string) (costIntegration string, found bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT cost_integration FROM traffic_sources WHERE id = $1 AND organization_id = $2`,
		trafficSourceID, orgID,
	).Scan(&costIntegration)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return costIntegration, true, nil
}

// SetTimezone records the ad account's IANA zone, learned by a sync
// (tech audit AN4). tz must already be a valid zone name.
func (r *Repository) SetTimezone(ctx context.Context, orgID, trafficSourceID, tz string) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE ad_account_connections SET timezone = $3
		WHERE organization_id = $1 AND traffic_source_id = $2 AND timezone <> $3`,
		orgID, trafficSourceID, tz); err != nil {
		return fmt.Errorf("adaccount: recording account timezone: %w", err)
	}
	return nil
}
