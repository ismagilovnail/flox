package streamset

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const streamSetColumns = `id, organization_id, campaign_id, name, priority, status, fallback_url, created_at, updated_at`

func scanStreamSet(row pgx.Row) (StreamSet, error) {
	var s StreamSet
	err := row.Scan(&s.ID, &s.OrganizationID, &s.CampaignID, &s.Name, &s.Priority, &s.Status, &s.FallbackURL, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// List returns every stream set for a campaign in evaluation order
// (priority) — the same ORDER BY routingstore.loadStreamSets uses,
// deliberately, since this is the same "first match wins" ordering an
// operator needs to see to understand their own configuration.
func (r *Repository) List(ctx context.Context, orgID, campaignID string) ([]StreamSet, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+streamSetColumns+`
		FROM stream_sets
		WHERE organization_id = $1 AND campaign_id = $2
		ORDER BY priority`,
		orgID, campaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: listing: %w", err)
	}
	defer rows.Close()

	sets := []StreamSet{}
	ids := []string{}
	for rows.Next() {
		s, err := scanStreamSet(rows)
		if err != nil {
			return nil, fmt.Errorf("streamset: scanning: %w", err)
		}
		sets = append(sets, s)
		ids = append(ids, s.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return sets, nil
	}

	trees, err := r.loadFilterTrees(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	flowsBySet, err := r.loadFlows(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	pixelsBySet, err := r.loadPixelIDs(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	for i := range sets {
		sets[i].RootFilter = trees[sets[i].ID]
		sets[i].Flows = flowsBySet[sets[i].ID]
		if sets[i].Flows == nil {
			sets[i].Flows = []Flow{}
		}
		sets[i].PixelIDs = pixelsBySet[sets[i].ID]
		if sets[i].PixelIDs == nil {
			sets[i].PixelIDs = []string{}
		}
	}
	return sets, nil
}

func (r *Repository) GetByID(ctx context.Context, orgID, campaignID, id string) (StreamSet, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+streamSetColumns+`
		FROM stream_sets
		WHERE id = $1 AND organization_id = $2 AND campaign_id = $3`,
		id, orgID, campaignID,
	)
	s, err := scanStreamSet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return StreamSet{}, apierror.NotFound("stream set not found")
	}
	if err != nil {
		return StreamSet{}, err
	}

	trees, err := r.loadFilterTrees(ctx, orgID, []string{id})
	if err != nil {
		return StreamSet{}, err
	}
	s.RootFilter = trees[id]

	flowsBySet, err := r.loadFlows(ctx, orgID, []string{id})
	if err != nil {
		return StreamSet{}, err
	}
	s.Flows = flowsBySet[id]
	if s.Flows == nil {
		s.Flows = []Flow{}
	}

	pixelsBySet, err := r.loadPixelIDs(ctx, orgID, []string{id})
	if err != nil {
		return StreamSet{}, err
	}
	s.PixelIDs = pixelsBySet[id]
	if s.PixelIDs == nil {
		s.PixelIDs = []string{}
	}
	return s, nil
}

// loadFilterTrees mirrors routingstore.loadFilterTrees's own two-query
// shape exactly (groups, then conditions, merged in Go) — deliberately:
// this is the read path's own established pattern for this schema, not a
// new one invented here. Conditions are appended before nested groups
// within a parent, same as routingstore's build() — AND/OR evaluation is
// commutative so this never affected routing correctness, and matching
// it here means a round-trip (load, edit, save) never surprises an
// operator with a reordered-looking tree they didn't touch.
func (r *Repository) loadFilterTrees(ctx context.Context, orgID string, streamSetIDs []string) (map[string]FilterNode, error) {
	groupRows, err := r.db.Query(ctx, `
		SELECT id, stream_set_id, parent_group_id, joiner
		FROM filter_groups
		WHERE stream_set_id = ANY($1) AND organization_id = $2
		ORDER BY position, id`,
		streamSetIDs, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: loading filter groups: %w", err)
	}
	defer groupRows.Close()

	type groupRow struct {
		id       string
		setID    string
		parentID *string
		joiner   routing.Joiner
	}
	var groups []groupRow
	for groupRows.Next() {
		var g groupRow
		if err := groupRows.Scan(&g.id, &g.setID, &g.parentID, &g.joiner); err != nil {
			return nil, fmt.Errorf("streamset: scanning filter group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return map[string]FilterNode{}, nil
	}

	groupIDs := make([]string, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.id)
	}

	condRows, err := r.db.Query(ctx, `
		SELECT filter_group_id, field, operator, value, value_to
		FROM filter_conditions
		WHERE filter_group_id = ANY($1) AND organization_id = $2
		ORDER BY position, id`,
		groupIDs, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: loading filter conditions: %w", err)
	}
	defer condRows.Close()

	condsByGroup := map[string][]FilterNode{}
	for condRows.Next() {
		var groupID string
		var n FilterNode
		n.Kind = NodeCondition
		if err := condRows.Scan(&groupID, &n.Field, &n.Operator, &n.Value, &n.ValueTo); err != nil {
			return nil, fmt.Errorf("streamset: scanning filter condition: %w", err)
		}
		condsByGroup[groupID] = append(condsByGroup[groupID], n)
	}
	if err := condRows.Err(); err != nil {
		return nil, err
	}

	childGroups := map[string][]groupRow{}
	roots := map[string]groupRow{}
	for _, g := range groups {
		if g.parentID == nil {
			roots[g.setID] = g
		} else {
			childGroups[*g.parentID] = append(childGroups[*g.parentID], g)
		}
	}

	var build func(g groupRow) FilterNode
	build = func(g groupRow) FilterNode {
		node := FilterNode{Kind: NodeGroup, Joiner: g.joiner, Children: []FilterNode{}}
		node.Children = append(node.Children, condsByGroup[g.id]...)
		for _, child := range childGroups[g.id] {
			node.Children = append(node.Children, build(child))
		}
		return node
	}

	trees := make(map[string]FilterNode, len(roots))
	for setID, root := range roots {
		trees[setID] = build(root)
	}
	return trees, nil
}

const flowStageColumns = `landing_enabled, landing_id, landing_as_pwa, pwa_enabled, pwa_id, pwa_type, postlanding_enabled, postlanding_id`

// scanFlowStages fills in the nullable landing/pwa/postlanding columns
// shared by loadFlows and loadFlowsTx's row-scanning.
func scanFlowStages(f *Flow, landingID, pwaID, pwaType, postlandingID *string) {
	f.Landing.LandingID = deref(landingID)
	f.Pwa.PwaID = deref(pwaID)
	f.Pwa.PwaType = PwaType(deref(pwaType))
	f.Postlanding.PostlandingID = deref(postlandingID)
}

// loadFlows mirrors routingstore.loadFlows's join shape but returns the
// CRUD-facing Flow (network/offer ids for re-editing) rather than the
// routing engine's resolved Destination (offer's live URL + active
// status) — a genuinely different concern (editing vs deciding).
func (r *Repository) loadFlows(ctx context.Context, orgID string, streamSetIDs []string) (map[string][]Flow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT stream_set_id, id, name, active, weight, `+flowStageColumns+`,
		       destination_kind, destination_network_id, destination_offer_id, destination_url
		FROM flows
		WHERE stream_set_id = ANY($1) AND organization_id = $2
		ORDER BY position, id`,
		streamSetIDs, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: loading flows: %w", err)
	}
	defer rows.Close()

	out := map[string][]Flow{}
	for rows.Next() {
		var (
			setID                             string
			f                                 Flow
			landingID, pwaID, pwaType, postID *string
			kind                              string
			networkID, offerID, url           *string
		)
		if err := rows.Scan(
			&setID, &f.ID, &f.Name, &f.Active, &f.Weight,
			&f.Landing.Enabled, &landingID, &f.Landing.AsPwa,
			&f.Pwa.Enabled, &pwaID, &pwaType,
			&f.Postlanding.Enabled, &postID,
			&kind, &networkID, &offerID, &url,
		); err != nil {
			return nil, fmt.Errorf("streamset: scanning flow: %w", err)
		}
		scanFlowStages(&f, landingID, pwaID, pwaType, postID)
		f.Destination = Destination{
			Kind:      routing.DestinationKind(kind),
			NetworkID: deref(networkID),
			OfferID:   deref(offerID),
			URL:       deref(url),
		}
		out[setID] = append(out[setID], f)
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Create inserts the stream set row, its filter tree, and its flows in
// one transaction — a partially-written stream set (row inserted, tree
// or flows didn't finish) would leave the routing engine reading a
// stream set with a broken or missing filter tree, silently matching
// everything or nothing.
func (r *Repository) Create(ctx context.Context, id, orgID, campaignID string, in CreateInput) (StreamSet, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return StreamSet{}, fmt.Errorf("streamset: beginning create tx: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		INSERT INTO stream_sets (id, organization_id, campaign_id, name, priority, fallback_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+streamSetColumns,
		id, orgID, campaignID, in.Name, in.Priority, in.FallbackURL,
	)
	s, err := scanStreamSet(row)
	if err != nil {
		return StreamSet{}, fmt.Errorf("streamset: inserting: %w", err)
	}

	if err := insertFilterGroup(ctx, tx, orgID, id, nil, 0, in.RootFilter); err != nil {
		return StreamSet{}, err
	}
	s.RootFilter = in.RootFilter

	flows, err := insertFlows(ctx, tx, orgID, id, in.Flows)
	if err != nil {
		return StreamSet{}, err
	}
	s.Flows = flows

	if err := insertPixelIDs(ctx, tx, orgID, id, in.PixelIDs); err != nil {
		return StreamSet{}, err
	}
	s.PixelIDs = in.PixelIDs
	if s.PixelIDs == nil {
		s.PixelIDs = []string{}
	}

	if err := tx.Commit(ctx); err != nil {
		return StreamSet{}, fmt.Errorf("streamset: committing create tx: %w", err)
	}
	return s, nil
}

// insertFilterGroup recursively inserts node (which must be a group —
// the root, or a nested group reached from a parent's Children) and its
// descendants. Conditions and nested groups keep separate position
// sequences among their own kind, matching each table's own "sibling
// conditions/groups under the same parent" ordering (00006) — see
// loadFilterTrees's doc comment for why conditions-before-groups on
// read is fine even though this doesn't preserve original interleaving.
func insertFilterGroup(ctx context.Context, tx pgx.Tx, orgID, streamSetID string, parentGroupID *string, position int, node FilterNode) error {
	if node.Kind != NodeGroup {
		return fmt.Errorf("streamset: insertFilterGroup called on a %s node", node.Kind)
	}
	groupID := idgen.New()
	_, err := tx.Exec(ctx, `
		INSERT INTO filter_groups (id, organization_id, stream_set_id, parent_group_id, joiner, position)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		groupID, orgID, streamSetID, parentGroupID, node.Joiner, position,
	)
	if err != nil {
		return fmt.Errorf("streamset: inserting filter group: %w", err)
	}

	condPos, groupPos := 0, 0
	for _, child := range node.Children {
		switch child.Kind {
		case NodeCondition:
			_, err := tx.Exec(ctx, `
				INSERT INTO filter_conditions (id, organization_id, filter_group_id, position, field, operator, value, value_to)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				idgen.New(), orgID, groupID, condPos, child.Field, child.Operator, child.Value, child.ValueTo,
			)
			if err != nil {
				return fmt.Errorf("streamset: inserting filter condition: %w", err)
			}
			condPos++
		case NodeGroup:
			if err := insertFilterGroup(ctx, tx, orgID, streamSetID, &groupID, groupPos, child); err != nil {
				return err
			}
			groupPos++
		}
	}
	return nil
}

// nullIfEmpty turns "" into a nil *string — the flows table's
// landing_id/pwa_id/pwa_type/postlanding_id columns are nullable, and
// pwa_type additionally has a CHECK constraint that only NULL (never an
// empty string) satisfies when the stage is unset.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func insertFlows(ctx context.Context, tx pgx.Tx, orgID, streamSetID string, in []FlowInput) ([]Flow, error) {
	flows := make([]Flow, len(in))
	for i, f := range in {
		flowID := idgen.New()
		var networkID, offerID, url *string
		if f.Destination.Kind == routing.DestinationOffer {
			networkID, offerID = &f.Destination.NetworkID, &f.Destination.OfferID
		} else {
			url = &f.Destination.URL
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO flows (
				id, organization_id, stream_set_id, name, active, weight, position,
				landing_enabled, landing_id, landing_as_pwa,
				pwa_enabled, pwa_id, pwa_type,
				postlanding_enabled, postlanding_id,
				destination_kind, destination_network_id, destination_offer_id, destination_url
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			flowID, orgID, streamSetID, f.Name, f.Active, f.Weight, i,
			f.Landing.Enabled, nullIfEmpty(f.Landing.LandingID), f.Landing.AsPwa,
			f.Pwa.Enabled, nullIfEmpty(f.Pwa.PwaID), nullIfEmpty(string(f.Pwa.PwaType)),
			f.Postlanding.Enabled, nullIfEmpty(f.Postlanding.PostlandingID),
			f.Destination.Kind, networkID, offerID, url,
		)
		if err != nil {
			return nil, fmt.Errorf("streamset: inserting flow: %w", err)
		}
		flows[i] = Flow{
			ID: flowID, Name: f.Name, Active: f.Active, Weight: f.Weight,
			Landing: f.Landing, Pwa: f.Pwa, Postlanding: f.Postlanding,
			Destination: f.Destination,
		}
	}
	return flows, nil
}

// Update applies only the non-nil fields in in. RootFilter/Flows, when
// present, replace the whole tree/array (delete-all, insert-all) inside
// the same transaction as the scalar-field update — matching
// internal/offer's own offer_links precedent and the frontend form's
// whole-tree/whole-array submission.
func (r *Repository) Update(ctx context.Context, orgID, campaignID, id string, in UpdateInput) (StreamSet, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return StreamSet{}, fmt.Errorf("streamset: beginning update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if in.Name != nil {
		sets = append(sets, "name = "+arg(*in.Name))
	}
	if in.Priority != nil {
		sets = append(sets, "priority = "+arg(*in.Priority))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}
	if in.FallbackURL != nil {
		sets = append(sets, "fallback_url = "+arg(*in.FallbackURL))
	}

	var s StreamSet
	if len(sets) > 0 {
		query := fmt.Sprintf(
			`UPDATE stream_sets SET %s WHERE id = %s AND organization_id = %s AND campaign_id = %s RETURNING %s`,
			joinComma(sets), arg(id), arg(orgID), arg(campaignID), streamSetColumns,
		)
		row := tx.QueryRow(ctx, query, args...)
		s, err = scanStreamSet(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return StreamSet{}, apierror.NotFound("stream set not found")
		}
		if err != nil {
			return StreamSet{}, fmt.Errorf("streamset: updating: %w", err)
		}
	} else {
		row := tx.QueryRow(ctx, `SELECT `+streamSetColumns+` FROM stream_sets WHERE id = $1 AND organization_id = $2 AND campaign_id = $3`, id, orgID, campaignID)
		s, err = scanStreamSet(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return StreamSet{}, apierror.NotFound("stream set not found")
		}
		if err != nil {
			return StreamSet{}, err
		}
	}

	if in.RootFilter != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM filter_groups WHERE stream_set_id = $1`, id); err != nil {
			return StreamSet{}, fmt.Errorf("streamset: clearing filter tree: %w", err)
		}
		if err := insertFilterGroup(ctx, tx, orgID, id, nil, 0, *in.RootFilter); err != nil {
			return StreamSet{}, err
		}
		s.RootFilter = *in.RootFilter
	} else {
		trees, err := r.loadFilterTreesTx(ctx, tx, orgID, id)
		if err != nil {
			return StreamSet{}, err
		}
		s.RootFilter = trees
	}

	if in.Flows != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM flows WHERE stream_set_id = $1`, id); err != nil {
			return StreamSet{}, fmt.Errorf("streamset: clearing flows: %w", err)
		}
		flows, err := insertFlows(ctx, tx, orgID, id, *in.Flows)
		if err != nil {
			return StreamSet{}, err
		}
		s.Flows = flows
	} else {
		flows, err := r.loadFlowsTx(ctx, tx, orgID, id)
		if err != nil {
			return StreamSet{}, err
		}
		s.Flows = flows
	}

	if in.PixelIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM stream_set_pixels WHERE stream_set_id = $1`, id); err != nil {
			return StreamSet{}, fmt.Errorf("streamset: clearing pixels: %w", err)
		}
		if err := insertPixelIDs(ctx, tx, orgID, id, *in.PixelIDs); err != nil {
			return StreamSet{}, err
		}
		s.PixelIDs = *in.PixelIDs
		if s.PixelIDs == nil {
			s.PixelIDs = []string{}
		}
	} else {
		pixelIDs, err := r.loadPixelIDsTx(ctx, tx, orgID, id)
		if err != nil {
			return StreamSet{}, err
		}
		s.PixelIDs = pixelIDs
	}

	if err := tx.Commit(ctx); err != nil {
		return StreamSet{}, fmt.Errorf("streamset: committing update tx: %w", err)
	}
	return s, nil
}

// loadFilterTreesTx/loadFlowsTx re-read the current tree/flows within an
// in-progress transaction, for the "this field wasn't touched by this
// PATCH" branch of Update — must see the tx's own view (which, absent a
// RootFilter/Flows in this PATCH, is identical to committed state, but
// reading through the same tx keeps this consistent under concurrent
// writes without relying on that).
func (r *Repository) loadFilterTreesTx(ctx context.Context, tx pgx.Tx, orgID, streamSetID string) (FilterNode, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, parent_group_id, joiner FROM filter_groups
		WHERE stream_set_id = $1 AND organization_id = $2
		ORDER BY position, id`,
		streamSetID, orgID,
	)
	if err != nil {
		return FilterNode{}, fmt.Errorf("streamset: reading filter groups in tx: %w", err)
	}
	defer rows.Close()

	type groupRow struct {
		id       string
		parentID *string
		joiner   routing.Joiner
	}
	var groups []groupRow
	var rootID string
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.id, &g.parentID, &g.joiner); err != nil {
			return FilterNode{}, fmt.Errorf("streamset: scanning filter group in tx: %w", err)
		}
		if g.parentID == nil {
			rootID = g.id
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return FilterNode{}, err
	}

	condRows, err := tx.Query(ctx, `
		SELECT filter_group_id, field, operator, value, value_to FROM filter_conditions fc
		JOIN filter_groups fg ON fg.id = fc.filter_group_id
		WHERE fg.stream_set_id = $1 AND fc.organization_id = $2
		ORDER BY fc.position, fc.id`,
		streamSetID, orgID,
	)
	if err != nil {
		return FilterNode{}, fmt.Errorf("streamset: reading filter conditions in tx: %w", err)
	}
	defer condRows.Close()

	condsByGroup := map[string][]FilterNode{}
	for condRows.Next() {
		var groupID string
		var n FilterNode
		n.Kind = NodeCondition
		if err := condRows.Scan(&groupID, &n.Field, &n.Operator, &n.Value, &n.ValueTo); err != nil {
			return FilterNode{}, fmt.Errorf("streamset: scanning filter condition in tx: %w", err)
		}
		condsByGroup[groupID] = append(condsByGroup[groupID], n)
	}
	if err := condRows.Err(); err != nil {
		return FilterNode{}, err
	}

	childGroups := map[string][]groupRow{}
	byID := map[string]groupRow{}
	for _, g := range groups {
		byID[g.id] = g
		if g.parentID != nil {
			childGroups[*g.parentID] = append(childGroups[*g.parentID], g)
		}
	}

	var build func(g groupRow) FilterNode
	build = func(g groupRow) FilterNode {
		node := FilterNode{Kind: NodeGroup, Joiner: g.joiner, Children: []FilterNode{}}
		node.Children = append(node.Children, condsByGroup[g.id]...)
		for _, child := range childGroups[g.id] {
			node.Children = append(node.Children, build(child))
		}
		return node
	}
	if rootID == "" {
		return FilterNode{}, nil
	}
	return build(byID[rootID]), nil
}

func (r *Repository) loadFlowsTx(ctx context.Context, tx pgx.Tx, orgID, streamSetID string) ([]Flow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, name, active, weight, `+flowStageColumns+`,
		       destination_kind, destination_network_id, destination_offer_id, destination_url
		FROM flows
		WHERE stream_set_id = $1 AND organization_id = $2
		ORDER BY position, id`,
		streamSetID, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: reading flows in tx: %w", err)
	}
	defer rows.Close()

	flows := []Flow{}
	for rows.Next() {
		var (
			f                                 Flow
			landingID, pwaID, pwaType, postID *string
			kind                              string
			networkID, offerID, url           *string
		)
		if err := rows.Scan(
			&f.ID, &f.Name, &f.Active, &f.Weight,
			&f.Landing.Enabled, &landingID, &f.Landing.AsPwa,
			&f.Pwa.Enabled, &pwaID, &pwaType,
			&f.Postlanding.Enabled, &postID,
			&kind, &networkID, &offerID, &url,
		); err != nil {
			return nil, fmt.Errorf("streamset: scanning flow in tx: %w", err)
		}
		scanFlowStages(&f, landingID, pwaID, pwaType, postID)
		f.Destination = Destination{Kind: routing.DestinationKind(kind), NetworkID: deref(networkID), OfferID: deref(offerID), URL: deref(url)}
		flows = append(flows, f)
	}
	return flows, rows.Err()
}

// insertPixelIDs attaches pixelIDs to streamSetID via stream_set_pixels
// (migration 00008) — a plain many-to-many junction row per id, no
// position/ordering column (it's a set, not a sequence). Callers pass
// already-org-validated ids (Service.checkPixelIDsBelongToOrg).
func insertPixelIDs(ctx context.Context, tx pgx.Tx, orgID, streamSetID string, pixelIDs []string) error {
	for _, pixelID := range pixelIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO stream_set_pixels (organization_id, stream_set_id, pixel_id) VALUES ($1, $2, $3)`,
			orgID, streamSetID, pixelID,
		); err != nil {
			return fmt.Errorf("streamset: inserting stream_set_pixels row: %w", err)
		}
	}
	return nil
}

// loadPixelIDs mirrors loadFlows' shape but for the far simpler
// stream_set_pixels junction table — one query, no joins. Ordered by
// pixel_id only for a deterministic (not meaningful) row order; the
// attachment is a set, not a sequence.
func (r *Repository) loadPixelIDs(ctx context.Context, orgID string, streamSetIDs []string) (map[string][]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT stream_set_id, pixel_id
		FROM stream_set_pixels
		WHERE stream_set_id = ANY($1) AND organization_id = $2
		ORDER BY pixel_id`,
		streamSetIDs, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: loading pixel attachments: %w", err)
	}
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var setID, pixelID string
		if err := rows.Scan(&setID, &pixelID); err != nil {
			return nil, fmt.Errorf("streamset: scanning pixel attachment: %w", err)
		}
		out[setID] = append(out[setID], pixelID)
	}
	return out, rows.Err()
}

func (r *Repository) loadPixelIDsTx(ctx context.Context, tx pgx.Tx, orgID, streamSetID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT pixel_id FROM stream_set_pixels
		WHERE stream_set_id = $1 AND organization_id = $2
		ORDER BY pixel_id`,
		streamSetID, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("streamset: reading pixel attachments in tx: %w", err)
	}
	defer rows.Close()

	pixelIDs := []string{}
	for rows.Next() {
		var pixelID string
		if err := rows.Scan(&pixelID); err != nil {
			return nil, fmt.Errorf("streamset: scanning pixel attachment in tx: %w", err)
		}
		pixelIDs = append(pixelIDs, pixelID)
	}
	return pixelIDs, rows.Err()
}

// Delete: every child table (filter_groups, filter_conditions, flows,
// stream_set_pixels) CASCADEs from stream_sets (00006/00008) — nothing
// else in the schema references stream_sets.id or flows.id as a FK
// target, so unlike network/offer/trafficsource's Delete, there is no
// 23503 case to guard against here.
func (r *Repository) Delete(ctx context.Context, orgID, campaignID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM stream_sets WHERE id = $1 AND organization_id = $2 AND campaign_id = $3`, id, orgID, campaignID)
	if err != nil {
		return fmt.Errorf("streamset: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("stream set not found")
	}
	return nil
}

// Reorder rewrites priority = index+1 for every id in orderedIDs, scoped
// to campaignID/orgID so an id from another campaign (or org) can't be
// smuggled into this campaign's priority sequence. One UPDATE per id
// rather than a bulk CASE statement — reorders are small (an operator's
// own stream set list, realistically under a few dozen) and this stays
// simple to read.
func (r *Repository) Reorder(ctx context.Context, orgID, campaignID string, orderedIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("streamset: beginning reorder tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, id := range orderedIDs {
		tag, err := tx.Exec(ctx,
			`UPDATE stream_sets SET priority = $1 WHERE id = $2 AND organization_id = $3 AND campaign_id = $4`,
			i+1, id, orgID, campaignID,
		)
		if err != nil {
			return fmt.Errorf("streamset: reordering: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apierror.Validation("invalid reorder", map[string]string{"orderedIds": fmt.Sprintf("stream set %q not found in this campaign", id)})
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("streamset: committing reorder tx: %w", err)
	}
	return nil
}

func (r *Repository) CampaignBelongsToOrg(ctx context.Context, orgID, campaignID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = $1 AND organization_id = $2)`, campaignID, orgID).Scan(&exists)
	return exists, err
}

func (r *Repository) NetworkBelongsToOrg(ctx context.Context, orgID, networkID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM networks WHERE id = $1 AND organization_id = $2)`, networkID, orgID).Scan(&exists)
	return exists, err
}

func (r *Repository) LandingBelongsToOrg(ctx context.Context, orgID, landingID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM landings WHERE id = $1 AND organization_id = $2)`, landingID, orgID).Scan(&exists)
	return exists, err
}

func (r *Repository) PwaBelongsToOrg(ctx context.Context, orgID, pwaID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pwas WHERE id = $1 AND organization_id = $2)`, pwaID, orgID).Scan(&exists)
	return exists, err
}

func (r *Repository) PostlandingBelongsToOrg(ctx context.Context, orgID, postlandingID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM postlandings WHERE id = $1 AND organization_id = $2)`, postlandingID, orgID).Scan(&exists)
	return exists, err
}

func (r *Repository) PixelBelongsToOrg(ctx context.Context, orgID, pixelID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pixels WHERE id = $1 AND organization_id = $2)`, pixelID, orgID).Scan(&exists)
	return exists, err
}

// OfferNetworkID returns the offer's own network_id if it belongs to
// orgID, so the service layer can derive Destination.NetworkID from the
// offer itself rather than trusting a client-supplied pair that could be
// mismatched — the flows table stores both ids denormalized (its own
// CHECK constraint requires it), but there is exactly one correct
// network for a given offer.
func (r *Repository) OfferNetworkID(ctx context.Context, orgID, offerID string) (string, bool, error) {
	var networkID string
	err := r.db.QueryRow(ctx, `SELECT network_id FROM offers WHERE id = $1 AND organization_id = $2`, offerID, orgID).Scan(&networkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return networkID, true, nil
}

func joinComma(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += ", " + p
	}
	return out
}
