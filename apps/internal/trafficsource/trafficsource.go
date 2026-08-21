// Package trafficsource is a deliberately narrow read-only slice of the
// traffic_sources entity — Phase 27's campaign form needs to resolve a
// source picker to a real traffic_source_id (CLAUDE.md #5: OrganizationID
// scopes it, never guessed), and nothing else does yet. Full traffic
// source CRUD (create/edit/archive a source) is its own, larger, not-yet-
// scheduled phase — see docs/frontend-integration.md for why this package
// stops at List rather than growing into that.
package trafficsource

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TrafficSource struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// List returns every traffic source for organizationID, ordered by name —
// the shape a picker/dropdown needs, nothing more.
func (r *Repository) List(ctx context.Context, organizationID string) ([]TrafficSource, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, type, status
		FROM traffic_sources
		WHERE organization_id = $1
		ORDER BY name`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("trafficsource: listing: %w", err)
	}
	defer rows.Close()

	out := []TrafficSource{}
	for rows.Next() {
		var s TrafficSource
		if err := rows.Scan(&s.ID, &s.Name, &s.Type, &s.Status); err != nil {
			return nil, fmt.Errorf("trafficsource: scanning: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trafficsource: reading: %w", err)
	}
	return out, nil
}
