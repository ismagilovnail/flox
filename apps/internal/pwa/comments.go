package pwa

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	maxComments       = 500
	commentAuthorMax  = 100
	commentTextMax    = 500
	commentReplyMax   = 500
	commentLikesMax   = 1000000
	commentTextMinLen = 1
)

var (
	geoCode  = regexp.MustCompile(`^[A-Z]{2}$`)
	langCode = regexp.MustCompile(`^[a-z]{2}$`)
)

// Comment is one hand-entered comment on a PWA's install page. Geo is an
// upper-case ISO country code, Lang a lower-case language code; together
// they name the comment set the entry belongs to.
type Comment struct {
	ID        string `json:"id"`
	Geo       string `json:"geo"`
	Lang      string `json:"lang"`
	Author    string `json:"author"`
	AvatarURL string `json:"avatarUrl"`
	Rating    int    `json:"rating"`
	Likes     int    `json:"likes"`
	// Date is the comment date the operator typed (YYYY-MM-DD), "" = none.
	Date  string `json:"date"`
	Text  string `json:"text"`
	Reply string `json:"reply"`
}

// CommentInput is a Comment as sent by the client; ids are assigned here.
type CommentInput struct {
	Geo       string `json:"geo"`
	Lang      string `json:"lang"`
	Author    string `json:"author"`
	AvatarURL string `json:"avatarUrl"`
	Rating    int    `json:"rating"`
	Likes     int    `json:"likes"`
	Date      string `json:"date"`
	Text      string `json:"text"`
	Reply     string `json:"reply"`
}

func (r *Repository) ListComments(ctx context.Context, orgID, pwaID string) ([]Comment, error) {
	if _, err := r.GetByID(ctx, orgID, pwaID); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, geo, lang, author, avatar_url, rating, likes, COALESCE(to_char(comment_date, 'YYYY-MM-DD'), ''), body, reply
		FROM pwa_comments
		WHERE organization_id = $1 AND pwa_id = $2
		ORDER BY position`, orgID, pwaID)
	if err != nil {
		return nil, fmt.Errorf("listing pwa comments: %w", err)
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.Geo, &c.Lang, &c.Author, &c.AvatarURL, &c.Rating, &c.Likes, &c.Date, &c.Text, &c.Reply); err != nil {
			return nil, fmt.Errorf("scanning pwa comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ReplaceComments swaps the PWA's whole comment list in one transaction.
func (r *Repository) ReplaceComments(ctx context.Context, orgID, pwaID string, in []CommentInput) ([]Comment, error) {
	if _, err := r.GetByID(ctx, orgID, pwaID); err != nil {
		return nil, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning comments tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM pwa_comments WHERE organization_id = $1 AND pwa_id = $2`, orgID, pwaID); err != nil {
		return nil, fmt.Errorf("clearing pwa comments: %w", err)
	}
	batch := &pgx.Batch{}
	out := make([]Comment, len(in))
	for i, c := range in {
		id := idgen.New()
		out[i] = Comment{ID: id, Geo: c.Geo, Lang: c.Lang, Author: c.Author, AvatarURL: c.AvatarURL, Rating: c.Rating,
			Likes: c.Likes, Date: c.Date, Text: c.Text, Reply: c.Reply}
		batch.Queue(`
			INSERT INTO pwa_comments (id, organization_id, pwa_id, geo, lang, author, avatar_url, rating, likes, comment_date, body, reply, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::date, $11, $12, $13)`,
			id, orgID, pwaID, c.Geo, c.Lang, c.Author, c.AvatarURL, c.Rating, c.Likes, c.Date, c.Text, c.Reply, i)
	}
	br := tx.SendBatch(ctx, batch)
	for range in {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return nil, fmt.Errorf("inserting pwa comment: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return nil, fmt.Errorf("closing comments batch: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing comments: %w", err)
	}
	return out, nil
}

func (s *Service) ListComments(ctx context.Context, orgID, pwaID string) ([]Comment, error) {
	return s.repo.ListComments(ctx, orgID, pwaID)
}

// ReplaceComments validates and normalises the list, then stores it.
func (s *Service) ReplaceComments(ctx context.Context, orgID, pwaID string, in []CommentInput) ([]Comment, error) {
	if len(in) > maxComments {
		return nil, apierror.Validation("invalid comments", map[string]string{"comments": fmt.Sprintf("at most %d comments", maxComments)})
	}
	cleaned := make([]CommentInput, len(in))
	for i, c := range in {
		c.Geo = strings.ToUpper(strings.TrimSpace(c.Geo))
		c.Lang = strings.ToLower(strings.TrimSpace(c.Lang))
		c.Author = strings.TrimSpace(c.Author)
		c.Text = strings.TrimSpace(c.Text)
		c.Reply = strings.TrimSpace(c.Reply)
		c.AvatarURL = strings.TrimSpace(c.AvatarURL)
		c.Date = strings.TrimSpace(c.Date)
		key := func(f string) string { return fmt.Sprintf("comments[%d].%s", i, f) }
		fields := map[string]string{}
		if !geoCode.MatchString(c.Geo) {
			fields[key("geo")] = "must be a two-letter country code"
		}
		if !langCode.MatchString(c.Lang) {
			fields[key("lang")] = "must be a two-letter language code"
		}
		if c.Author == "" || utf8.RuneCountInString(c.Author) > commentAuthorMax {
			fields[key("author")] = fmt.Sprintf("required, up to %d characters", commentAuthorMax)
		}
		if c.Rating < 1 || c.Rating > 5 {
			fields[key("rating")] = "must be between 1 and 5"
		}
		if n := utf8.RuneCountInString(c.Text); n < commentTextMinLen || n > commentTextMax {
			fields[key("text")] = fmt.Sprintf("required, up to %d characters", commentTextMax)
		}
		if c.AvatarURL != "" && !isValidURL(c.AvatarURL) {
			fields[key("avatarUrl")] = "must be an http(s) URL"
		}
		if c.Likes < 0 || c.Likes > commentLikesMax {
			fields[key("likes")] = fmt.Sprintf("must be between 0 and %d", commentLikesMax)
		}
		if c.Date != "" {
			if _, err := time.Parse("2006-01-02", c.Date); err != nil {
				fields[key("date")] = "must be a date as YYYY-MM-DD"
			}
		}
		if utf8.RuneCountInString(c.Reply) > commentReplyMax {
			fields[key("reply")] = fmt.Sprintf("up to %d characters", commentReplyMax)
		}
		if len(fields) > 0 {
			return nil, apierror.Validation("invalid comments", fields)
		}
		cleaned[i] = c
	}
	return s.repo.ReplaceComments(ctx, orgID, pwaID, cleaned)
}
