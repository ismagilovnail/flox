package classifier_test

import (
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
)

func TestParseUserAgent(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want classifier.UAResult
	}{
		{
			name: "iPhone Safari",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
			want: classifier.UAResult{Device: "mobile", Platform: "ios", OS: "ios", OSVersion: "17.4", Browser: "safari", BrowserVersion: "17.4"},
		},
		{
			name: "iPad Safari",
			ua:   "Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
			want: classifier.UAResult{Device: "tablet", Platform: "ios", OS: "ios", OSVersion: "17.4", Browser: "safari", BrowserVersion: "17.4"},
		},
		{
			name: "Android Chrome phone",
			ua:   "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
			want: classifier.UAResult{Device: "mobile", Platform: "android", OS: "android", OSVersion: "14", Browser: "chrome", BrowserVersion: "124.0.0.0"},
		},
		{
			name: "Android Chrome tablet",
			ua:   "Mozilla/5.0 (Linux; Android 14; SM-X200) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			want: classifier.UAResult{Device: "tablet", Platform: "android", OS: "android", OSVersion: "14", Browser: "chrome", BrowserVersion: "124.0.0.0"},
		},
		{
			name: "Windows Chrome desktop",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			want: classifier.UAResult{Device: "desktop", Platform: "windows", OS: "windows", OSVersion: "10.0", Browser: "chrome", BrowserVersion: "124.0.0.0"},
		},
		{
			name: "Windows Edge desktop",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
			want: classifier.UAResult{Device: "desktop", Platform: "windows", OS: "windows", OSVersion: "10.0", Browser: "edge", BrowserVersion: "124.0.0.0"},
		},
		{
			name: "macOS Safari desktop",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
			want: classifier.UAResult{Device: "desktop", Platform: "macos", OS: "macos", OSVersion: "10.15.7", Browser: "safari", BrowserVersion: "17.4"},
		},
		{
			name: "macOS Firefox desktop",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:125.0) Gecko/20100101 Firefox/125.0",
			want: classifier.UAResult{Device: "desktop", Platform: "macos", OS: "macos", OSVersion: "10.15", Browser: "firefox", BrowserVersion: "125.0"},
		},
		{
			name: "Linux Chrome desktop",
			ua:   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			want: classifier.UAResult{Device: "desktop", Platform: "linux", OS: "linux", OSVersion: "", Browser: "chrome", BrowserVersion: "124.0.0.0"},
		},
		{
			name: "Samsung Internet",
			ua:   "Mozilla/5.0 (Linux; Android 14; SM-S928B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/25.0 Chrome/115.0.0.0 Mobile Safari/537.36",
			want: classifier.UAResult{Device: "mobile", Platform: "android", OS: "android", OSVersion: "14", Browser: "samsung_internet", BrowserVersion: "25.0"},
		},
		{
			name: "unknown/empty UA -> everything empty, not guessed",
			ua:   "",
			want: classifier.UAResult{},
		},
		{
			name: "unrecognized bespoke UA -> device defaults to desktop (no mobile/tablet marker), os/platform stay empty (no safe default), browser buckets to other",
			ua:   "SomeInternalTool/3.1",
			want: classifier.UAResult{Device: "desktop", Browser: "other"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.ParseUserAgent(tc.ua)
			if got != tc.want {
				t.Fatalf("ParseUserAgent(%q) = %+v, want %+v", tc.ua, got, tc.want)
			}
		})
	}
}
