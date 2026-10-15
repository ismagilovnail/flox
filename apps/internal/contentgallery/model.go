// Package contentgallery is the Content Gallery: a tenant-scoped media
// library with a flat folder structure, files in object storage
// (apps/internal/storage), like the reference tracker's
// (https://adset.pro/guide_ru/galereya-kontenta).
//
// Folders don't nest (the reference's own folder panel doesn't either —
// just a flat list you toggle between). Deleting a folder moves its files
// to the root rather than deleting them (migration 00027's ON DELETE SET
// NULL). Files soft-delete to an "archived" status first, restorable via
// the Active/Inactive toggle the reference itself has; permanent deletion
// (Service.DeleteFiles) is gated to files that are already archived —
// Repository.DeleteFiles' own WHERE clause enforces this, not just a
// service-layer check, so there is no code path that can hard-delete an
// active file directly. Bulk operations are capped at 50 ids at a time,
// matching the reference's own documented selection limit.
//
// Video is intentionally NOT supported yet — the reference transcodes
// uploaded video server-side (a "Processing..." status before a preview
// exists), which needs an ffmpeg/worker pipeline this repo doesn't have.
// Migration 00027's file_type CHECK already allows 'video' so adding it
// later needs no second migration, but Service.UploadFile rejects a video
// upload today with a clear, honest error rather than accepting one it
// can't actually process.
package contentgallery

import "time"

type FileType string

const (
	FileTypeImage FileType = "image"
	FileTypeVideo FileType = "video"
)

type FileStatus string

const (
	FileStatusActive   FileStatus = "active"
	FileStatusArchived FileStatus = "archived"
)

type Folder struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type File struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	FolderID         *string    `json:"folderId"`
	UploadedByUserID string     `json:"uploadedByUserId"`
	FileType         FileType   `json:"fileType"`
	OriginalFilename string     `json:"originalFilename"`
	MimeType         string     `json:"mimeType"`
	SizeBytes        int64      `json:"sizeBytes"`
	Status           FileStatus `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`

	// URL is a permanent, public download URL computed by Service at
	// request time from ObjectKey — never stored as its own column
	// (apps/internal/storage.Client.PublicURL's own doc explains why
	// Content Gallery files are public-read rather than presigned: a
	// picked file can end up as a PWA's iconUrl, which has to keep
	// resolving for as long as that PWA stays installed, not just for a
	// presigned URL's expiry window).
	URL string `json:"url"`

	// ObjectKey is the raw storage key — never serialized, a client has no
	// legitimate use for it, only the presigned URL above.
	ObjectKey string `json:"-"`

	// A video's poster (its first frame, made by the uploading browser),
	// duration and frame size — display only; nil/"" when unknown (the
	// browser could not decode it) and for images.
	PosterURL       string `json:"posterUrl,omitempty"`
	DurationMs      *int   `json:"durationMs,omitempty"`
	Width           *int   `json:"width,omitempty"`
	Height          *int   `json:"height,omitempty"`
	PosterObjectKey string `json:"-"`
}

// ListFilesParams' FolderID is tri-state, matching the reference's own
// "All / this folder / root only" folder filter: nil means no folder
// filter at all (every file regardless of folder); a pointer to ""
// means root only (folder_id IS NULL); a pointer to a real id means
// exactly that folder.
type ListFilesParams struct {
	FolderID         *string
	FileType         FileType   // "" = any
	Status           FileStatus // "" defaults to FileStatusActive in the repository
	UploadedByUserID string     // "" = any
	Limit            int
	Offset           int
}

type ListFilesResult struct {
	Files []File `json:"files"`
	Total int    `json:"total"`
}
