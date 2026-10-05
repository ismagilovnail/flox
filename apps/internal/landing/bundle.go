package landing

import (
	"archive/zip"
	"fmt"
	"io"
	"mime"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

const (
	// maxArchiveBytes caps the uploaded .zip itself; maxUnpackedBytes caps
	// the sum of its files' declared uncompressed sizes (zip-bomb guard —
	// archive/zip itself errors if a file's real size differs from its
	// declared one, so the declared sum is trustworthy). maxEntryBytes
	// caps the entry file, which lives in Postgres.
	maxArchiveBytes  = 50 << 20
	maxUnpackedBytes = 100 << 20
	maxEntryBytes    = 2 << 20
	maxBundleFiles   = 1000
	maxWarnings      = 20
)

// entryFiles are the accepted entry-file names, in preference order when
// an archive carries both.
var entryFiles = []string{"index.html", "index.twig"}

// bundle is a validated ZIP: the entry file's source plus every other
// file, keyed by its path relative to the bundle root.
type bundle struct {
	EntryFile string
	EntryHTML string
	Assets    []bundleAsset
	FileCount int   // assets + the entry file
	TotalSize int64 // uncompressed, entry file included
	Warnings  []string
}

type bundleAsset struct {
	Path        string
	ContentType string
	file        *zip.File
}

func (a bundleAsset) Open() (io.ReadCloser, error) { return a.file.Open() }
func (a bundleAsset) Size() int64                  { return int64(a.file.UncompressedSize64) }

func archiveError(msg string) error {
	return apierror.Validation("invalid archive", map[string]string{"archive": msg})
}

// parseBundle validates an uploaded ZIP and resolves its layout. The entry
// file must sit at the archive root — or, as a convenience for the very
// common "compressed the folder, not its contents" archive, at the root of
// the archive's single top-level folder, which is then stripped.
func parseBundle(ra io.ReaderAt, size int64) (*bundle, error) {
	if size <= 0 {
		return nil, archiveError("the archive is empty")
	}
	if size > maxArchiveBytes {
		return nil, archiveError(fmt.Sprintf("the archive exceeds the %dMB limit", maxArchiveBytes>>20))
	}
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, archiveError("the file is not a valid ZIP archive")
	}

	type entry struct {
		name string
		file *zip.File
	}
	var files []entry
	var total uint64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, ok := cleanArchivePath(f.Name)
		if !ok {
			return nil, archiveError(fmt.Sprintf("unsafe path in archive: %q", f.Name))
		}
		if isJunkPath(name) {
			continue
		}
		total += f.UncompressedSize64
		if total > maxUnpackedBytes {
			return nil, archiveError(fmt.Sprintf("the archive unpacks to more than %dMB", maxUnpackedBytes>>20))
		}
		files = append(files, entry{name: name, file: f})
		if len(files) > maxBundleFiles {
			return nil, archiveError(fmt.Sprintf("the archive contains more than %d files", maxBundleFiles))
		}
	}

	byName := make(map[string]*zip.File, len(files))
	for _, e := range files {
		byName[e.name] = e.file
	}

	root, entryName := findEntry(byName)
	if entryName == "" {
		return nil, archiveError("the archive must contain index.html or index.twig at its root")
	}

	b := &bundle{EntryFile: entryName}
	for _, e := range files {
		rel := strings.TrimPrefix(e.name, root)
		if root != "" && !strings.HasPrefix(e.name, root) {
			// Unreachable: findEntry only picks a root every file shares.
			continue
		}
		b.FileCount++
		b.TotalSize += int64(e.file.UncompressedSize64)
		if rel == entryName {
			src, err := readEntry(e.file)
			if err != nil {
				return nil, err
			}
			b.EntryHTML = src
			continue
		}
		b.Assets = append(b.Assets, bundleAsset{Path: rel, ContentType: contentTypeFor(rel), file: e.file})
	}

	b.Warnings = absoluteAssetRefs(b.EntryHTML)
	return b, nil
}

// findEntry returns the bundle root ("" or "folder/") and the entry file
// name found there, or "" when neither layout applies.
func findEntry(byName map[string]*zip.File) (root, entry string) {
	for _, name := range entryFiles {
		if _, ok := byName[name]; ok {
			return "", name
		}
	}
	// Single top-level folder: every file must share it.
	top := ""
	for name := range byName {
		i := strings.IndexByte(name, '/')
		if i < 0 {
			return "", ""
		}
		dir := name[:i+1]
		if top == "" {
			top = dir
		} else if dir != top {
			return "", ""
		}
	}
	if top == "" {
		return "", ""
	}
	for _, name := range entryFiles {
		if _, ok := byName[top+name]; ok {
			return top, name
		}
	}
	return "", ""
}

// cleanArchivePath normalizes a ZIP entry name and rejects anything that
// could escape the bundle prefix once used as an object key (zip-slip):
// absolute paths, drive letters, and ".." segments.
func cleanArchivePath(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
		return "", false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", false
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// isJunkPath drops OS metadata archivers add on their own (macOS's
// __MACOSX resource forks and .DS_Store, Windows' Thumbs.db).
func isJunkPath(name string) bool {
	if strings.HasPrefix(name, "__MACOSX/") || strings.Contains(name, "/__MACOSX/") {
		return true
	}
	base := path.Base(name)
	return base == ".DS_Store" || base == "Thumbs.db"
}

func readEntry(f *zip.File) (string, error) {
	if f.UncompressedSize64 > maxEntryBytes {
		return "", archiveError(fmt.Sprintf("the entry file exceeds the %dMB limit", maxEntryBytes>>20))
	}
	rc, err := f.Open()
	if err != nil {
		return "", archiveError("the entry file could not be read")
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes+1))
	if err != nil {
		return "", archiveError("the entry file could not be read")
	}
	if !utf8.Valid(data) {
		return "", archiveError("the entry file must be UTF-8 text")
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", archiveError("the entry file is empty")
	}
	return string(data), nil
}

// extraTypes covers the asset types landings commonly ship that Go's
// built-in mime table may not know on a minimal host (no /etc/mime.types).
var extraTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",
	".mp4":   "video/mp4",
	".webm":  "video/webm",
	".mp3":   "audio/mpeg",
	".ogg":   "audio/ogg",
	".wav":   "audio/wav",
	".html":  "text/html; charset=utf-8",
	".htm":   "text/html; charset=utf-8",
	".txt":   "text/plain; charset=utf-8",
}

func contentTypeFor(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := extraTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// absoluteRefPattern finds root-absolute references ("/style.css", not
// "//cdn.example/x.js" and not "style.css") in src/href attributes and
// CSS url(...) — they resolve against the serving host, not the bundle.
var absoluteRefPattern = regexp.MustCompile(`(?i)(?:\b(?:src|href)\s*=\s*["']|url\(\s*["']?)(/[^/"')\s][^"')\s]*)`)

func absoluteAssetRefs(html string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range absoluteRefPattern.FindAllStringSubmatch(html, -1) {
		ref := m[1]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
		if len(out) == maxWarnings {
			break
		}
	}
	return out
}
