// Package storage is the one S3-compatible object-storage client this repo
// has (config.S3Config), used for the content gallery, landing bundles and
// assets. Built against minio-go,
// which speaks the S3 API generically — it works the same against the
// dev/test stack's own minio container (infra/docker-compose.*.yml) and
// against a real S3-compatible provider in production, no code change.
package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/ismagilovnail/flox/apps/internal/config"
)

type Client struct {
	mc             *minio.Client // talks to cfg.Endpoint — used for actual object I/O
	bucket         string
	publicEndpoint string // cfg.PublicEndpoint, trailing slash trimmed — see PublicURL
}

// NewClient dials endpoint but performs no network I/O itself (minio.New
// only builds a client value) — matches this repo's existing "connect,
// then have the caller verify reachability" split for optional
// infrastructure (chconn.NewConn, rediscache.NewClient). Call EnsureBucket
// to actually confirm the endpoint/bucket are reachable.
func NewClient(cfg config.S3Config) (*Client, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: S3_ENDPOINT and S3_BUCKET are required")
	}

	secure := strings.HasPrefix(cfg.Endpoint, "https://")
	host := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	mc, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: building client: %w", err)
	}

	publicEndpoint := cfg.PublicEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.Endpoint
	}

	return &Client{mc: mc, bucket: cfg.Bucket, publicEndpoint: strings.TrimSuffix(publicEndpoint, "/")}, nil
}

// publicReadPolicy grants anonymous GetObject on every object in the
// bucket — deliberate, not an oversight: Content Gallery files are meant
// to end up embedded in things end users' browsers fetch unauthenticated
// (a PWA's installed manifest icon, a landing page's banner image), so
// they need a URL that keeps resolving indefinitely, the same way the
// former mock's own cdn.floxlink.io URLs implied. A presigned, expiring
// URL was tried first and rejected: it would work in the gallery's own
// browsing UI but silently break a few hours after being saved into e.g.
// a PWA's iconUrl, which has to keep resolving for as long as that PWA
// stays installed. The object key's org-id prefix plus a ULID keeps a
// listing/enumeration attack impractical, but this is intentionally NOT
// tenant-access-controlled at the storage layer — organization_id scoping
// for who can *see a file in the gallery UI* still lives entirely in
// Postgres (content_gallery_files), same as every other tenant boundary
// in this repo (§36-TENANCY): this policy only affects "can a URL you
// already have be fetched," not "can you list/browse another org's
// files."
func publicReadPolicy(bucket string) string {
	return fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"AWS": ["*"]},
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`, bucket)
}

// EnsureBucket confirms the configured bucket exists (creating it if not)
// and carries the public-read policy above — called once at startup,
// best-effort like every other optional dependency (apps/api/main.go logs
// a warning and carries on if this fails, the same "a down dependency
// degrades its own route group, not the whole API" stance ClickHouse/
// Redis already use there).
//
// No bucket-CORS call here, deliberately: apps/web's cropper (image-crop-
// dialog.tsx) draws an *existing* Content Gallery file onto a <canvas> —
// e.g. attaching an already-uploaded image as a PWA icon — which needs
// CORS response headers or the canvas is "tainted" (every read back off
// it, toBlob included, throws SecurityError). The S3-compatible bucket-
// CORS API this package would otherwise call here (minio-go's
// SetBucketCors) turned out unimplemented on self-hosted MinIO — it
// returns NotImplemented — because MinIO instead serves CORS off its own
// server-level `api.cors_allow_origin` admin config, which already
// defaults to "*" (confirmed live: a GET with an Origin header comes back
// with a matching Access-Control-Allow-Origin with zero extra setup).
// Calling an API the server rejects just to duplicate a default it
// already applies isn't worth carrying. A real S3-compatible provider in
// production needs CORS configured on the bucket through its own
// console/CLI/infra-as-code if this behavior is wanted there — a
// reasonable one-time operational step, not something this process
// should silently mutate on every boot.
func (c *Client) EnsureBucket(ctx context.Context, region string) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("storage: checking bucket: %w", err)
	}
	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return fmt.Errorf("storage: creating bucket: %w", err)
		}
	}
	if err := c.mc.SetBucketPolicy(ctx, c.bucket, publicReadPolicy(c.bucket)); err != nil {
		return fmt.Errorf("storage: setting bucket policy: %w", err)
	}
	return nil
}

// PutObject uploads r (exactly size bytes) to key, replacing anything
// already there. Callers pass a size known up front (multipart file
// headers already carry one) rather than buffering to compute it.
func (c *Client) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("storage: uploading %s: %w", key, err)
	}
	return nil
}

// Open streams key's object — for a file only the API may hand out, such
// as an organization's data export (internal/orgexport). The caller
// closes it.
func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: opening %s: %w", key, err)
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("storage: opening %s: %w", key, err)
	}
	return obj, nil
}

// DeleteObject is best-effort cleanup only — callers that fail to remove
// an object after e.g. a since-rolled-back DB write log and move on
// (an orphaned object costs storage, not correctness; a dangling DB row
// pointing at a missing object would be the real bug).
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: deleting %s: %w", key, err)
	}
	return nil
}

// PublicURL builds key's permanent, unsigned URL — no network call, no
// error, no expiry (see EnsureBucket's own doc for why files are public-
// read). Uses cfg.PublicEndpoint, the host a *browser* can reach, which
// in the dev/test docker compose stacks differs from the in-network host
// this process itself talks to (config.S3Config.PublicEndpoint's own doc
// has the full split).
func (c *Client) PublicURL(key string) string {
	return c.publicEndpoint + "/" + c.bucket + "/" + key
}

// CopyPrefix copies every object under srcPrefix to the same relative key
// under dstPrefix, server-side (no bytes pass through this process) —
// content types and other metadata carry over. First consumer:
// landing.Service.Duplicate, so a duplicated bundle never shares objects
// with its source (re-uploading one must not change the other).
func (c *Client) CopyPrefix(ctx context.Context, srcPrefix, dstPrefix string) error {
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: srcPrefix, Recursive: true}) {
		if obj.Err != nil {
			return fmt.Errorf("storage: listing %s: %w", srcPrefix, obj.Err)
		}
		dst := dstPrefix + strings.TrimPrefix(obj.Key, srcPrefix)
		if _, err := c.mc.CopyObject(ctx,
			minio.CopyDestOptions{Bucket: c.bucket, Object: dst},
			minio.CopySrcOptions{Bucket: c.bucket, Object: obj.Key},
		); err != nil {
			return fmt.Errorf("storage: copying %s: %w", obj.Key, err)
		}
	}
	return nil
}

// DeletePrefix removes every object under prefix. Best-effort cleanup,
// same contract as DeleteObject.
func (c *Client) DeletePrefix(ctx context.Context, prefix string) error {
	if prefix == "" {
		return fmt.Errorf("storage: refusing to delete an empty prefix")
	}
	objects := make(chan minio.ObjectInfo)
	listDone := make(chan struct{})
	var listErr error
	go func() {
		defer close(listDone)
		defer close(objects)
		for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if obj.Err != nil {
				listErr = obj.Err
				return
			}
			select {
			case objects <- obj:
			case <-ctx.Done():
				return
			}
		}
	}()
	var firstErr error
	for res := range c.mc.RemoveObjects(ctx, c.bucket, objects, minio.RemoveObjectsOptions{}) {
		if res.Err != nil && firstErr == nil {
			firstErr = res.Err
		}
	}
	<-listDone
	if listErr != nil {
		return fmt.Errorf("storage: listing %s: %w", prefix, listErr)
	}
	if firstErr != nil {
		return fmt.Errorf("storage: deleting under %s: %w", prefix, firstErr)
	}
	return nil
}
