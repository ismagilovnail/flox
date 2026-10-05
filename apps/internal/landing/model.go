// Package landing implements the Landing API (§28): handler → service →
// repository. Two kinds of landing exist: TypeExternal points at a URL the
// advertiser already hosts; TypeInternal is an uploaded ZIP bundle — its
// entry file (index.html / index.twig) lives in Postgres so the template
// editor can change it in place, and every other file in the archive is
// unpacked into object storage (docs/landings.md, "ZIP bundles").
package landing

import (
	"io"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusArchived:
		return true
	default:
		return false
	}
}

type Type string

const (
	TypeInternal Type = "internal"
	TypeExternal Type = "external"
)

func (t Type) Valid() bool {
	switch t {
	case TypeInternal, TypeExternal:
		return true
	default:
		return false
	}
}

type Landing struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	// Title is the page's display title (the reference product shows it
	// e.g. as a PWA header); LogoURL a square logo used in listings.
	Title   string `json:"title"`
	LogoURL string `json:"logoUrl"`
	Type    Type   `json:"type"`
	// URL is the advertiser's own URL for TypeExternal. For TypeInternal
	// it's the public base URL the bundle's assets are served from —
	// derived at read time from BundlePrefix (Service.decorate), never
	// stored, so a changed S3_PUBLIC_ENDPOINT can't leave stale values.
	URL string `json:"url"`
	// Content is the entry file's source (TypeInternal only).
	Content string `json:"content"`
	// EntryFile is "index.html" or "index.twig" — whichever the uploaded
	// archive carried at its root (TypeInternal only).
	EntryFile string `json:"entryFile"`
	// FileCount/BundleSize describe the uploaded archive's contents
	// (entry file included), for display only.
	FileCount  int   `json:"fileCount"`
	BundleSize int64 `json:"bundleSize"`
	// BundlePrefix is the object-storage key prefix of the current upload
	// ("landings/{org}/{id}/{version}"), empty when the landing has no
	// assets. Internal bookkeeping, never sent to clients.
	BundlePrefix string    `json:"-"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// CreateInput is the JSON create path — TypeExternal only. Internal
// landings are created by CreateFromArchive.
type CreateInput struct {
	Name    string
	Title   string
	LogoURL string
	Type    Type
	URL     string
	Status  Status // optional; "" means the DB default (active)
}

// ArchiveCreateInput creates a TypeInternal landing from an uploaded ZIP.
type ArchiveCreateInput struct {
	Name    string
	Title   string
	LogoURL string
	Status  Status
	Archive Archive
}

// Archive is an uploaded ZIP — a multipart.File already satisfies
// io.ReaderAt, which archive/zip needs to read the central directory.
type Archive struct {
	ReaderAt io.ReaderAt
	Size     int64
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching network.UpdateInput. Type is
// accepted only so a client resending the current type isn't rejected;
// changing it is (Service.Update).
type UpdateInput struct {
	Name    *string
	Title   *string
	LogoURL *string
	Type    *Type
	URL     *string
	Content *string
	Status  *Status
}

// UploadResult is what an archive upload returns: the landing plus any
// root-absolute asset references found in the entry file ("/style.css"),
// which won't resolve against the bundle's base URL — surfaced as
// warnings, not errors, since the reference product accepts such archives
// too (they just render broken).
type UploadResult struct {
	Landing  Landing  `json:"landing"`
	Warnings []string `json:"warnings"`
}
