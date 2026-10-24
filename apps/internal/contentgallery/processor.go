package contentgallery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/storage"
)

const (
	// ProcessorInterval is how often apps/worker looks for videos to
	// transcode when there were none.
	ProcessorInterval = 5 * time.Second
	// processingLease is how long a claimed video is the claiming worker's;
	// past it (the worker died) another takes it. Longer than
	// processingTimeout.
	processingLease = 20 * time.Minute
	// processingTimeout bounds one video's whole run.
	processingTimeout = 15 * time.Minute
	// maxProcessingAttempts: a video taken this many times without an
	// outcome (it keeps killing the worker) is given up on.
	maxProcessingAttempts = 3
)

// Processor transcodes the gallery's source videos (docs/content-gallery.md
// "Transcoding"), one at a time, in apps/worker.
type Processor struct {
	Repo    *Repository
	Storage *storage.Client
	Media   Media
	Logger  *slog.Logger
	// TempDir is where sources and results are written (os.TempDir() if
	// empty); each job gets its own directory, removed afterwards.
	TempDir string
	// OrgID limits the processor to one organization's videos — tests
	// only, so they leave the shared dev database's rows alone; "" (the
	// worker) is every organization.
	OrgID string
}

// RunLoop processes videos until ctx ends: straight on while there are
// some, every interval otherwise.
func (p *Processor) RunLoop(ctx context.Context, interval time.Duration) {
	for {
		did, err := p.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			p.Logger.Error("gallery video transcoding", "error", err)
		}
		if did && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// RunOnce transcodes one waiting video. did: one was taken. An error is
// transient (storage, database): the file stays processing and is taken
// again once its lease runs out. A video the tools refuse is not an error:
// it is marked failed.
func (p *Processor) RunOnce(ctx context.Context) (did bool, err error) {
	job, found, err := p.Repo.claimProcessing(ctx, processingLease, p.OrgID)
	if err != nil || !found {
		return false, err
	}
	if job.Attempts > maxProcessingAttempts {
		return true, p.Repo.failProcessing(ctx, job, fmt.Sprintf("gave up after %d attempts", maxProcessingAttempts))
	}
	runCtx, cancel := context.WithTimeout(ctx, processingTimeout)
	defer cancel()
	out, err := p.transcode(runCtx, job)
	var mErr *mediaError
	switch {
	case errors.As(err, &mErr):
		p.Logger.Info("gallery video transcoding failed", "file_id", job.ID, "reason", mErr.Reason)
		return true, p.Repo.failProcessing(ctx, job, mErr.Reason)
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
		return true, p.Repo.failProcessing(ctx, job, fmt.Sprintf("took longer than %d minutes", int(processingTimeout.Minutes())))
	case err != nil:
		return true, err
	}
	stored, err := p.Repo.finishProcessing(ctx, job, out)
	if err != nil || !stored {
		// Not stored (deleted meanwhile, or the database failed): the
		// objects just made belong to nothing.
		_ = p.Storage.DeleteObject(ctx, out.ObjectKey)
		_ = p.Storage.DeleteObject(ctx, out.PosterKey)
		return true, err
	}
	if err := p.Storage.DeleteObject(ctx, job.SourceKey); err != nil {
		p.Logger.Warn("gallery video transcoding: source left in storage", "file_id", job.ID, "error", err)
	}
	p.Logger.Info("gallery video transcoded", "file_id", job.ID, "size", out.Size, "duration_ms", out.DurationMs)
	return true, nil
}

// transcode downloads job's source, makes the MP4 and its poster within the
// gallery's limits and uploads both.
func (p *Processor) transcode(ctx context.Context, job processingJob) (transcoded, error) {
	dir, err := os.MkdirTemp(p.TempDir, "flox-video-")
	if err != nil {
		return transcoded{}, fmt.Errorf("contentgallery: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "source"+filepath.Ext(job.SourceKey))
	if err := p.Storage.DownloadToFile(ctx, job.SourceKey, src); err != nil {
		return transcoded{}, err
	}
	in, err := p.Media.Probe(ctx, src)
	if err != nil {
		return transcoded{}, err
	}
	if !in.HasVideo {
		return transcoded{}, &mediaError{Reason: "the file has no video stream"}
	}
	if in.DurationMs > maxTranscodeDurationMs {
		return transcoded{}, &mediaError{Reason: fmt.Sprintf("the video is longer than %d minutes", maxTranscodeDurationMs/60000)}
	}

	mp4 := filepath.Join(dir, "video.mp4")
	if err := p.Media.ToMP4(ctx, src, mp4); err != nil {
		return transcoded{}, err
	}
	st, err := os.Stat(mp4)
	if err != nil {
		return transcoded{}, fmt.Errorf("contentgallery: reading the MP4: %w", err)
	}
	if st.Size() > maxUploadBytes {
		return transcoded{}, &mediaError{Reason: fmt.Sprintf("the MP4 is %.1fMB, over the %dMB limit — shorten the video and upload it again",
			float64(st.Size())/(1<<20), maxUploadBytes>>20)}
	}
	info, err := p.Media.Probe(ctx, mp4)
	if err != nil {
		return transcoded{}, err
	}
	poster := filepath.Join(dir, "poster.jpg")
	at := 0.5
	if info.DurationMs > 0 && info.DurationMs < 1000 {
		at = float64(info.DurationMs) / 2000
	}
	if err := p.Media.Poster(ctx, mp4, poster, at); err != nil {
		return transcoded{}, err
	}

	out := transcoded{
		ObjectKey: job.OrganizationID + "/" + job.ID + ".mp4", PosterKey: job.OrganizationID + "/" + job.ID + "-poster.jpg",
		Size: st.Size(), DurationMs: info.DurationMs, Width: info.Width, Height: info.Height,
	}
	if err := p.upload(ctx, mp4, out.ObjectKey, "video/mp4"); err != nil {
		return transcoded{}, err
	}
	if err := p.upload(ctx, poster, out.PosterKey, "image/jpeg"); err != nil {
		_ = p.Storage.DeleteObject(ctx, out.ObjectKey)
		return transcoded{}, err
	}
	return out, nil
}

func (p *Processor) upload(ctx context.Context, path, key, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("contentgallery: opening %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("contentgallery: reading %s: %w", filepath.Base(path), err)
	}
	return p.Storage.PutObject(ctx, key, f, st.Size(), contentType)
}
