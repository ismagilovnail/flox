package contentgallery

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sniffSource takes each format's own first bytes and nothing else.
func TestSniffSource(t *testing.T) {
	ftyp := []byte("\x00\x00\x00\x14ftypqt  ")
	oldQT := []byte("\x00\x00\x00\x08wide\x00\x00")
	avi := []byte("RIFF\x00\x00\x00\x00AVI LIST")
	wav := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	mkv := []byte("\x1a\x45\xdf\xa3\x9f\x42\x86\x81")
	for _, tc := range []struct {
		ext  string
		head []byte
		want bool
	}{
		{".mov", ftyp, true}, {".m4v", ftyp, true}, {".3gp", ftyp, true}, {".mov", oldQT, true},
		{".avi", avi, true}, {".mkv", mkv, true},
		{".mov", []byte("whatever"), false}, {".mov", []byte("\x00\x00"), false},
		{".avi", wav, false}, {".mkv", ftyp, false}, {".avi", mkv, false}, {".flv", ftyp, false},
	} {
		if got := sniffSource(tc.ext, tc.head); got != tc.want {
			t.Errorf("sniffSource(%s, %q) = %v, want %v", tc.ext, tc.head, got, tc.want)
		}
	}
}

// The tools get no environment (audit SEC-7): the worker's holds the
// database password and the storage and encryption keys.
func TestToolsRunWithoutTheWorkersEnvironment(t *testing.T) {
	env, err := exec.LookPath("env")
	if err != nil {
		t.Skip("no env binary")
	}
	t.Setenv("FLOX_TEST_SECRET", "s3cr3t")
	out, err := FFmpeg{}.run(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("the tool's environment = %q, want empty", out)
	}
}

// Each file is read with the one demuxer its extension names, and nothing
// else is allowed (audit SEC-7).
func TestInputArgsForceOneDemuxer(t *testing.T) {
	for path, want := range map[string]string{"/t/source.MOV": "mov", "/t/source.3gp": "mov", "/t/video.mp4": "mov", "/t/source.avi": "avi", "/t/source.mkv": "matroska", "/t/poster.jpg": "jpeg_pipe"} {
		args, err := inputArgs(path)
		if err != nil {
			t.Fatalf("inputArgs(%s): %v", path, err)
		}
		if got := strings.Join(args, " "); got != "-protocol_whitelist file -format_whitelist "+want+" -f "+want {
			t.Errorf("inputArgs(%s) = %q", path, got)
		}
	}
	var mErr *mediaError
	if _, err := inputArgs("/t/list.m3u8"); !errors.As(err, &mErr) {
		t.Fatalf("inputArgs(m3u8) = %v, want a mediaError", err)
	}
}

// The real tools: a rotated 2 s MOV with sound becomes an upright H.264
// MP4 within the side limit, with a poster; a file that is not a video is
// a mediaError (failed, not retried). Skipped where ffmpeg is not
// installed — apps/worker's image has it.
func TestFFmpegTranscodesAMOV(t *testing.T) {
	ffmpeg, err1 := exec.LookPath("ffmpeg")
	ffprobe, err2 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil {
		t.Skip("ffmpeg/ffprobe not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mov")
	// 2560x1440 landscape, then tagged as rotated 90° (a phone held
	// upright) without re-encoding.
	plain := filepath.Join(dir, "plain.mov")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=2560x1440:rate=25:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "mpeg4", "-c:a", "pcm_s16le", "-shortest", plain).CombinedOutput(); err != nil {
		t.Fatalf("making the MOV: %v\n%s", err, out)
	}
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-display_rotation", "90", "-i", plain, "-c", "copy", src).CombinedOutput(); err != nil {
		// ffmpeg before 6.1 has no -display_rotation: the old rotate tag.
		if out2, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-i", plain, "-c", "copy", "-metadata:s:v:0", "rotate=90", src).CombinedOutput(); err != nil {
			t.Fatalf("rotating the MOV: %v\n%s\n%s", err, out, out2)
		}
	}
	m := FFmpeg{FFmpegPath: ffmpeg, FFprobePath: ffprobe, Threads: 2}

	in, err := m.Probe(ctx, src)
	if err != nil || !in.HasVideo || in.Width != 1440 || in.Height != 2560 || in.DurationMs < 1900 || in.DurationMs > 2100 {
		t.Fatalf("Probe(source) = %+v, %v; want upright 1440x2560, ~2 s", in, err)
	}
	mp4 := filepath.Join(dir, "video.mp4")
	if err := m.ToMP4(ctx, src, mp4); err != nil {
		t.Fatalf("ToMP4: %v", err)
	}
	out, err := m.Probe(ctx, mp4)
	if err != nil || out.Width != 1080 || out.Height != 1920 || out.DurationMs < 1900 || out.DurationMs > 2100 {
		t.Fatalf("Probe(mp4) = %+v, %v; want 1080x1920 (long side capped, upright), ~2 s", out, err)
	}
	codecs, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", mp4).Output()
	if err != nil || string(codecs) != "h264\naac\n" {
		t.Fatalf("MP4 streams = %q, %v; want h264 and aac", codecs, err)
	}
	poster := filepath.Join(dir, "poster.jpg")
	if err := m.Poster(ctx, mp4, poster, 0.5); err != nil {
		t.Fatalf("Poster: %v", err)
	}
	if p, err := m.Probe(ctx, poster); err != nil || p.Width != 360 || p.Height != 640 {
		t.Fatalf("Probe(poster) = %+v, %v; want 360x640", p, err)
	}

	// AVI and MKV through their own demuxers.
	for _, ext := range []string{".avi", ".mkv"} {
		other := filepath.Join(dir, "other"+ext)
		if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=1",
			"-c:v", "mpeg4", other).CombinedOutput(); err != nil {
			t.Fatalf("making the %s: %v\n%s", ext, err, out)
		}
		if err := m.ToMP4(ctx, other, filepath.Join(dir, "other"+ext+".mp4")); err != nil {
			t.Fatalf("ToMP4(%s): %v", ext, err)
		}
	}
	// A playlist named .mov that points at a local file is read as MOV
	// only, and refused (audit SEC-7).
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	playlist := filepath.Join(dir, "playlist.mov")
	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:1.0,\nfile://"+secret+"\n#EXT-X-ENDLIST\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var plErr *mediaError
	if err := m.ToMP4(ctx, playlist, filepath.Join(dir, "playlist.mp4")); !errors.As(err, &plErr) {
		t.Fatalf("ToMP4(playlist) = %v, want a mediaError", err)
	}

	junk := filepath.Join(dir, "junk.mov")
	if err := os.WriteFile(junk, []byte("\x00\x00\x00\x14ftypqt  not a real movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mErr *mediaError
	if err := m.ToMP4(ctx, junk, filepath.Join(dir, "junk.mp4")); !errors.As(err, &mErr) || mErr.Reason == "" {
		t.Fatalf("ToMP4(junk) = %v, want a mediaError with a reason", err)
	}
}
