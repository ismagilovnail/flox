package backup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/ismagilovnail/flox/apps/internal/config"
)

const (
	objectsDir          = "objects"
	objectsManifestsDir = "objects-manifests"
	// drillSample is how many mirrored files the drill downloads from
	// both sides and hashes.
	drillSample = 20
)

var minioRemove = minio.RemoveObjectOptions{}

// ObjectsManifest describes one mirror run.
type ObjectsManifest struct {
	Name       string    `json:"name"`
	Bucket     string    `json:"bucket"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Objects    int       `json:"objects"`
	Bytes      int64     `json:"bytes"`
	Copied     int       `json:"copied"`
}

// Objects mirrors the files bucket into the backup bucket. It adds and
// updates; a file deleted from the source stays in the mirror (an
// accidental delete is what the mirror is for). Pruning the mirror is the
// backup bucket's lifecycle rule, if any.
type Objects struct {
	src       *minio.Client
	srcBucket string
	bucket    *bucket
}

func NewObjects(src config.S3Config, t Target) (*Objects, error) {
	c, err := newClient(src.Endpoint, src.Region, src.AccessKeyID, src.SecretAccessKey)
	if err != nil {
		return nil, err
	}
	b, err := newBucket(t)
	if err != nil {
		return nil, err
	}
	return &Objects{src: c, srcBucket: src.Bucket, bucket: b}, nil
}

// mirrored is what the mirror holds under objects/, by source key.
func (o *Objects) mirrored(ctx context.Context) (map[string]minio.ObjectInfo, error) {
	out := map[string]minio.ObjectInfo{}
	for obj := range o.bucket.c.ListObjects(ctx, o.bucket.name, minio.ListObjectsOptions{Prefix: objectsDir + "/", Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("backup: listing the mirror: %w", obj.Err)
		}
		out[strings.TrimPrefix(obj.Key, objectsDir+"/")] = obj
	}
	return out, nil
}

// Mirror copies every new or changed file and writes the run's manifest.
func (o *Objects) Mirror(ctx context.Context, at time.Time) (ObjectsManifest, error) {
	m := ObjectsManifest{Name: name(at), Bucket: o.srcBucket, StartedAt: at.UTC()}
	have, err := o.mirrored(ctx)
	if err != nil {
		return m, err
	}
	for obj := range o.src.ListObjects(ctx, o.srcBucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return m, fmt.Errorf("backup: listing %s: %w", o.srcBucket, obj.Err)
		}
		m.Objects++
		m.Bytes += obj.Size
		if h, ok := have[obj.Key]; ok && h.Size == obj.Size && !obj.LastModified.After(h.LastModified) {
			continue
		}
		if err := o.copy(ctx, obj); err != nil {
			return m, err
		}
		m.Copied++
	}
	m.FinishedAt = time.Now().UTC()
	return m, o.bucket.putJSON(ctx, objectsManifestsDir+"/"+m.Name+".json", m)
}

// copy streams one file across: the two buckets may be on different
// endpoints, so no server-side copy.
func (o *Objects) copy(ctx context.Context, obj minio.ObjectInfo) error {
	r, err := o.src.GetObject(ctx, o.srcBucket, obj.Key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("backup: reading %s: %w", obj.Key, err)
	}
	defer r.Close()
	if _, err := o.bucket.c.PutObject(ctx, o.bucket.name, objectsDir+"/"+obj.Key, r, obj.Size,
		minio.PutObjectOptions{ContentType: obj.ContentType}); err != nil {
		return fmt.Errorf("backup: mirroring %s: %w", obj.Key, err)
	}
	return nil
}

// Latest is the newest mirror run.
func (o *Objects) Latest(ctx context.Context) (ObjectsManifest, error) {
	var m ObjectsManifest
	_, err := o.bucket.latest(ctx, objectsManifestsDir, &m)
	return m, err
}

// Drill checks that every file the source had at the last mirror run is
// in the mirror with its size, and that a sample of them hash the same on
// both sides.
func (o *Objects) Drill(ctx context.Context) []Check {
	m, err := o.Latest(ctx)
	if err != nil {
		return []Check{fail(StoreObjects, "latest mirror", err)}
	}
	checks := []Check{pass(StoreObjects, "latest mirror", fmt.Sprintf("%s, %d files", m.Name, m.Objects))}
	have, err := o.mirrored(ctx)
	if err != nil {
		return append(checks, fail(StoreObjects, "mirror", err))
	}
	var missing []string
	var present []minio.ObjectInfo
	for obj := range o.src.ListObjects(ctx, o.srcBucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return append(checks, fail(StoreObjects, "mirror", obj.Err))
		}
		if obj.LastModified.After(m.StartedAt) {
			continue // added or changed after that run
		}
		if h, ok := have[obj.Key]; !ok || h.Size != obj.Size {
			missing = append(missing, obj.Key)
			continue
		}
		present = append(present, obj)
	}
	if len(missing) > 0 {
		checks = append(checks, Check{Store: StoreObjects, Name: "mirror",
			Detail: fmt.Sprintf("%d files missing or differing, e.g. %s", len(missing), missing[0])})
	} else {
		checks = append(checks, pass(StoreObjects, "mirror", fmt.Sprintf("%d files present", len(present))))
	}

	rand.Shuffle(len(present), func(i, j int) { present[i], present[j] = present[j], present[i] })
	present = present[:min(drillSample, len(present))]
	for _, obj := range present {
		a, err := hashObject(ctx, o.src, o.srcBucket, obj.Key)
		if err != nil {
			return append(checks, fail(StoreObjects, "sample", err))
		}
		b, err := hashObject(ctx, o.bucket.c, o.bucket.name, objectsDir+"/"+obj.Key)
		if err != nil {
			return append(checks, fail(StoreObjects, "sample", err))
		}
		if a != b {
			return append(checks, Check{Store: StoreObjects, Name: "sample", Detail: obj.Key + ": content differs"})
		}
	}
	return append(checks, pass(StoreObjects, "sample", fmt.Sprintf("%d files hash the same", len(present))))
}

func hashObject(ctx context.Context, c *minio.Client, bucket, key string) (string, error) {
	r, err := c.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}
	defer r.Close()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("reading %s/%s: %w", bucket, key, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
