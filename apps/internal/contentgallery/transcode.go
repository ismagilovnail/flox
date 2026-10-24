package contentgallery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Media is what transcoding needs of a video tool: FFmpeg is the real one;
// tests may fake it.
type Media interface {
	// Probe reads a file's first video stream (after rotation) and duration.
	Probe(ctx context.Context, path string) (VideoInfo, error)
	// ToMP4 writes src as an MP4 (H.264/AAC, long side at most
	// maxTranscodeSide, metadata stripped, faststart) to dst.
	ToMP4(ctx context.Context, src, dst string) error
	// Poster writes a JPEG frame of src at second at, long side 640, to dst.
	Poster(ctx context.Context, src, dst string, at float64) error
}

// VideoInfo is a probed video. HasVideo false: no video stream at all.
type VideoInfo struct {
	HasVideo      bool
	DurationMs    int
	Width, Height int
}

const (
	// maxTranscodeSide bounds the MP4's long side (1080p either way).
	maxTranscodeSide = 1920
	// maxTranscodeDuration bounds a source's length: CPU time is the
	// worker's, and a gallery video is a creative, not a film.
	maxTranscodeDurationMs = 10 * 60 * 1000
)

// FFmpeg runs the ffmpeg and ffprobe binaries (FFMPEG_PATH, FFPROBE_PATH).
type FFmpeg struct {
	FFmpegPath, FFprobePath string
	// Threads bounds ffmpeg's threads, so one video does not take every
	// core of the worker.
	Threads int
}

// mediaError is a video the tools refused (not a transient failure): the
// file is marked failed with Reason.
type mediaError struct{ Reason string }

func (e *mediaError) Error() string { return e.Reason }

// demuxers names the one ffmpeg demuxer each file the worker opens is read
// with (audit SEC-7): forced with -f and the only one allowed
// (-format_whitelist), so a crafted file cannot pull in another demuxer —
// an HLS or concat playlist that reads local files or the network — and
// only local files may be opened (-protocol_whitelist file). The sources
// (transcodeExt), the MP4 made from them, its poster.
var demuxers = map[string]string{
	".mov": "mov", ".m4v": "mov", ".3gp": "mov", ".mp4": "mov",
	".avi": "avi", ".mkv": "matroska",
	".jpg": "jpeg_pipe",
}

// inputArgs are the options that come before "-i path".
func inputArgs(path string) ([]string, error) {
	demuxer, ok := demuxers[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return nil, &mediaError{Reason: "unsupported video format"}
	}
	return []string{"-protocol_whitelist", "file", "-format_whitelist", demuxer, "-f", demuxer}, nil
}

// run starts a tool with an empty environment (audit SEC-7): it parses
// files people uploaded, and the worker's environment holds the database
// password and the storage and encryption keys. The binaries are absolute
// paths (exec.LookPath in apps/worker), so no PATH is needed.
func (f FFmpeg) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = []string{}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, &mediaError{Reason: "the video could not be read: " + lastLine(stderr.String())}
		}
		return nil, fmt.Errorf("contentgallery: running %s: %w", bin, err)
	}
	return stdout.Bytes(), nil
}

// lastLine is the tool's last diagnostic line — its actual complaint.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if len(line) > 300 {
		line = line[:300]
	}
	if line == "" {
		return "unknown error"
	}
	return line
}

func (f FFmpeg) Probe(ctx context.Context, path string) (VideoInfo, error) {
	in, err := inputArgs(path)
	if err != nil {
		return VideoInfo{}, err
	}
	args := append([]string{"-v", "error"}, in...)
	out, err := f.run(ctx, f.FFprobePath, append(args, "-print_format", "json", "-show_format", "-show_streams", path)...)
	if err != nil {
		return VideoInfo{}, err
	}
	var p struct {
		Streams []struct {
			CodecType    string                  `json:"codec_type"`
			Width        int                     `json:"width"`
			Height       int                     `json:"height"`
			Tags         struct{ Rotate string } `json:"tags"`
			SideDataList []struct {
				Rotation float64 `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		return VideoInfo{}, &mediaError{Reason: "the video could not be read"}
	}
	var info VideoInfo
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil && d > 0 {
		info.DurationMs = int(math.Round(d * 1000))
	}
	for _, s := range p.Streams {
		if s.CodecType != "video" {
			continue
		}
		info.HasVideo, info.Width, info.Height = true, s.Width, s.Height
		rotation, _ := strconv.ParseFloat(s.Tags.Rotate, 64)
		for _, sd := range s.SideDataList {
			if sd.Rotation != 0 {
				rotation = sd.Rotation
			}
		}
		if r := int(math.Abs(rotation)) % 180; r == 90 { // shown upright: sides swap
			info.Width, info.Height = info.Height, info.Width
		}
		break
	}
	return info, nil
}

func (f FFmpeg) threads() string {
	if f.Threads <= 0 {
		return "2"
	}
	return strconv.Itoa(f.Threads)
}

func (f FFmpeg) ToMP4(ctx context.Context, src, dst string) error {
	in, err := inputArgs(src)
	if err != nil {
		return err
	}
	side := strconv.Itoa(maxTranscodeSide)
	args := append([]string{"-hide_banner", "-nostdin", "-y"}, in...)
	_, err = f.run(ctx, f.FFmpegPath, append(args, "-i", src,
		"-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1",
		"-vf", "scale=w='min(iw,"+side+")':h='min(ih,"+side+")':force_original_aspect_ratio=decrease:force_divisible_by=2",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-movflags", "+faststart", "-threads", f.threads(), dst)...)
	return err
}

func (f FFmpeg) Poster(ctx context.Context, src, dst string, at float64) error {
	in, err := inputArgs(src)
	if err != nil {
		return err
	}
	args := append([]string{"-hide_banner", "-nostdin", "-y", "-ss", strconv.FormatFloat(at, 'f', 3, 64)}, in...)
	_, err = f.run(ctx, f.FFmpegPath, append(args, "-i", src,
		"-frames:v", "1", "-vf", "scale=w='min(iw,640)':h='min(ih,640)':force_original_aspect_ratio=decrease",
		"-q:v", "4", "-threads", f.threads(), dst)...)
	return err
}
