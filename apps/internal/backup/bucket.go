package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// errNone: nothing of that kind in the bucket yet.
var errNone = errors.New("backup: none yet")

func newClient(endpoint, region, accessKeyID, secretAccessKey string) (*minio.Client, error) {
	secure := strings.HasPrefix(endpoint, "https://")
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	c, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("backup: s3 client for %s: %w", endpoint, err)
	}
	return c, nil
}

// bucket is the backup target.
type bucket struct {
	c    *minio.Client
	name string
}

func newBucket(t Target) (*bucket, error) {
	c, err := newClient(t.Endpoint, t.Region, t.AccessKeyID, t.SecretAccessKey)
	if err != nil {
		return nil, err
	}
	return &bucket{c: c, name: t.Bucket}, nil
}

// ensure creates the bucket if it is not there (a fresh dev stack; in
// production it is made, with its lifecycle and access, beforehand).
func (b *bucket) ensure(ctx context.Context) error {
	ok, err := b.c.BucketExists(ctx, b.name)
	if err != nil {
		return fmt.Errorf("backup: checking bucket %s: %w", b.name, err)
	}
	if ok {
		return nil
	}
	if err := b.c.MakeBucket(ctx, b.name, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("backup: creating bucket %s: %w", b.name, err)
	}
	return nil
}

func (b *bucket) putJSON(ctx context.Context, key string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if _, err := b.c.PutObject(ctx, b.name, key, bytes.NewReader(body), int64(len(body)),
		minio.PutObjectOptions{ContentType: "application/json"}); err != nil {
		return fmt.Errorf("backup: writing %s: %w", key, err)
	}
	return nil
}

func (b *bucket) getJSON(ctx context.Context, key string, v any) error {
	obj, err := b.c.GetObject(ctx, b.name, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("backup: reading %s: %w", key, err)
	}
	defer obj.Close()
	body, err := io.ReadAll(obj)
	if err != nil {
		return fmt.Errorf("backup: reading %s: %w", key, err)
	}
	return json.Unmarshal(body, v)
}

// names lists the <name>.json files under dir, oldest first.
func (b *bucket) names(ctx context.Context, dir string) ([]string, error) {
	var out []string
	for o := range b.c.ListObjects(ctx, b.name, minio.ListObjectsOptions{Prefix: dir + "/"}) {
		if o.Err != nil {
			return nil, fmt.Errorf("backup: listing %s: %w", dir, o.Err)
		}
		if n, ok := strings.CutSuffix(path.Base(o.Key), ".json"); ok {
			out = append(out, n)
		}
	}
	sort.Strings(out) // names are timestamps
	return out, nil
}

// latest reads the newest <name>.json under dir into v.
func (b *bucket) latest(ctx context.Context, dir string, v any) (string, error) {
	all, err := b.names(ctx, dir)
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", errNone
	}
	n := all[len(all)-1]
	return n, b.getJSON(ctx, dir+"/"+n+".json", v)
}

// removePrefix deletes every object under prefix.
func (b *bucket) removePrefix(ctx context.Context, prefix string) error {
	var listed []minio.ObjectInfo
	for o := range b.c.ListObjects(ctx, b.name, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if o.Err != nil {
			return fmt.Errorf("backup: listing %s: %w", prefix, o.Err)
		}
		listed = append(listed, o)
	}
	objs := make(chan minio.ObjectInfo, len(listed))
	for _, o := range listed {
		objs <- o
	}
	close(objs)
	for e := range b.c.RemoveObjects(ctx, b.name, objs, minio.RemoveObjectsOptions{}) {
		if e.Err != nil {
			return fmt.Errorf("backup: removing %s: %w", e.ObjectName, e.Err)
		}
	}
	return nil
}

// oldest returns the names beyond the newest keep.
func oldest(names []string, keep int) []string {
	if len(names) <= keep {
		return nil
	}
	return names[:len(names)-keep]
}
