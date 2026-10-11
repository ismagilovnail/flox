package pixel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

type Repository struct {
	db *pgxpool.Pool
	// key encrypts access tokens (ParseKey).
	key []byte
}

func NewRepository(db *pgxpool.Pool, key []byte) *Repository { return &Repository{db: db, key: key} }

// selectColumns never includes access_token: only whether one is set and
// its last 4 characters leave the database through Pixel.
const selectColumns = `id, organization_id, name, provider, pixel_id, events,
	access_token <> '', access_token_last4, test_event_code, url, event_names,
	status, created_at, updated_at`

func scanPixel(row pgx.Row) (Pixel, error) {
	var p Pixel
	var names []byte
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Provider, &p.PixelID, &p.Events,
		&p.HasAccessToken, &p.AccessTokenLast4, &p.TestEventCode, &p.URL, &names,
		&p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.EventNames = map[string]string{}
	if err := json.Unmarshal(names, &p.EventNames); err != nil {
		return p, fmt.Errorf("pixel: decoding event_names: %w", err)
	}
	return p, nil
}

// sealToken returns the stored ciphertext and last 4 characters of token;
// "" stays "" (no token).
func (r *Repository) sealToken(token string) (sealed, last4 string, err error) {
	if token == "" {
		return "", "", nil
	}
	sealed, err = encrypt(r.key, token)
	if err != nil {
		return "", "", err
	}
	last4 = token
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}
	return sealed, last4, nil
}

func encodeNames(names map[string]string) ([]byte, error) {
	if names == nil {
		names = map[string]string{}
	}
	return json.Marshal(names)
}

func (r *Repository) List(ctx context.Context, organizationID string, page pagination.Page) ([]Pixel, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM pixels
		WHERE organization_id = $1
		ORDER BY name, id
		LIMIT $2 OFFSET $3`,
		organizationID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("pixel: listing: %w", err)
	}
	defer rows.Close()

	out := []Pixel{}
	for rows.Next() {
		p, err := scanPixel(rows)
		if err != nil {
			return nil, fmt.Errorf("pixel: scanning: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Pixel, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+selectColumns+`
		FROM pixels
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	)
	p, err := scanPixel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pixel{}, apierror.NotFound("pixel not found")
	}
	return p, err
}

func (r *Repository) Create(ctx context.Context, id, orgID string, in CreateInput) (Pixel, error) {
	sealed, last4, err := r.sealToken(in.AccessToken)
	if err != nil {
		return Pixel{}, err
	}
	names, err := encodeNames(in.EventNames)
	if err != nil {
		return Pixel{}, err
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO pixels (id, organization_id, name, provider, pixel_id, events,
			access_token, access_token_last4, test_event_code, url, event_names)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+selectColumns,
		id, orgID, in.Name, in.Provider, in.PixelID, in.Events,
		sealed, last4, in.TestEventCode, in.URL, names,
	)
	return scanPixel(row)
}

// copyToken gives pixel dst the access token of src, still encrypted —
// Duplicate copies a pixel without ever decrypting its token.
func (r *Repository) copyToken(ctx context.Context, orgID, srcID, dstID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE pixels d SET access_token = s.access_token, access_token_last4 = s.access_token_last4
		FROM pixels s
		WHERE d.id = $1 AND d.organization_id = $3 AND s.id = $2 AND s.organization_id = $3`,
		dstID, srcID, orgID)
	if err != nil {
		return fmt.Errorf("pixel: copying access token: %w", err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, orgID, id string, in UpdateInput) (Pixel, error) {
	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Provider != nil {
		sets = append(sets, "provider = "+arg(*in.Provider))
	}
	if in.PixelID != nil {
		sets = append(sets, "pixel_id = "+arg(*in.PixelID))
	}
	if in.Events != nil {
		sets = append(sets, "events = "+arg(*in.Events))
	}
	if in.AccessToken != nil {
		sealed, last4, err := r.sealToken(*in.AccessToken)
		if err != nil {
			return Pixel{}, err
		}
		sets = append(sets, "access_token = "+arg(sealed), "access_token_last4 = "+arg(last4))
	}
	if in.TestEventCode != nil {
		sets = append(sets, "test_event_code = "+arg(*in.TestEventCode))
	}
	if in.URL != nil {
		sets = append(sets, "url = "+arg(*in.URL))
	}
	if in.EventNames != nil {
		names, err := encodeNames(*in.EventNames)
		if err != nil {
			return Pixel{}, err
		}
		sets = append(sets, "event_names = "+arg(names))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	query := fmt.Sprintf(
		`UPDATE pixels SET %s WHERE id = %s AND organization_id = %s RETURNING %s`,
		strings.Join(sets, ", "), arg(id), arg(orgID), selectColumns,
	)

	row := r.db.QueryRow(ctx, query, args...)
	p, err := scanPixel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pixel{}, apierror.NotFound("pixel not found")
	}
	return p, err
}

// Delete: stream_set_pixels.pixel_id (00008) CASCADEs from pixels, so no
// FK conflict is possible there — unlike landing/pwa/postlanding's
// RESTRICT-guarded flows.*_id columns, deleting a pixel just drops its
// stream-set attachments along with it. No defensive 23503 catch needed,
// unlike those siblings' Delete.
func (r *Repository) Delete(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM pixels WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("deleting pixel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("pixel not found")
	}
	return nil
}

// Count is how many pixels rows the organization has — List's total.
func (r *Repository) Count(ctx context.Context, organizationID string) (int, error) {
	return pagination.Count(ctx, r.db, "pixels", organizationID)
}

// Credentials reads a pixel for sending, its access token decrypted.
func (r *Repository) Credentials(ctx context.Context, orgID, id string) (Credentials, error) {
	var c Credentials
	var sealed string
	var names []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, provider, pixel_id, access_token, test_event_code, url, event_names, status
		FROM pixels
		WHERE id = $1 AND organization_id = $2`,
		id, orgID,
	).Scan(&c.ID, &c.Provider, &c.PixelID, &sealed, &c.TestEventCode, &c.URL, &names, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, apierror.NotFound("pixel not found")
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("pixel: reading credentials: %w", err)
	}
	if err := json.Unmarshal(names, &c.EventNames); err != nil {
		return Credentials{}, fmt.Errorf("pixel: decoding event_names: %w", err)
	}
	if sealed != "" {
		if c.AccessToken, err = decrypt(r.key, sealed); err != nil {
			return Credentials{}, err
		}
	}
	return c, nil
}

// Deliveries is the pixel's delivery log, newest first, at most limit rows.
func (r *Repository) Deliveries(ctx context.Context, orgID, pixelID string, limit int) ([]Delivery, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, event_type, click_id, delivery_status, attempt_count, response_status_code,
		       message, created_at, updated_at
		FROM pixel_deliveries
		WHERE organization_id = $1 AND pixel_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3`,
		orgID, pixelID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pixel: listing deliveries: %w", err)
	}
	defer rows.Close()

	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.EventType, &d.ClickID, &d.Status, &d.AttemptCount,
			&d.ResponseStatusCode, &d.Message, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("pixel: scanning delivery: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
