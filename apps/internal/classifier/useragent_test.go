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
			name: "Opera desktop",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 OPR/110.0.0.0",
			want: classifier.UAResult{Device: "desktop", Platform: "windows", OS: "windows", OSVersion: "10.0", Browser: "opera", BrowserVersion: "110.0.0.0"},
		},
		{
			name: "Opera Mini iOS",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) OPiOS/16.0.15.124050 Mobile/15E148 Safari/9537.53",
			want: classifier.UAResult{Device: "mobile", Platform: "ios", OS: "ios", OSVersion: "17.4", Browser: "opera", BrowserVersion: "16.0.15.124050"},
		},
		{
			name: "Yandex Browser",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 YaBrowser/24.4.0.0 Safari/537.36",
			want: classifier.UAResult{Device: "desktop", Platform: "windows", OS: "windows", OSVersion: "10.0", Browser: "yandex", BrowserVersion: "24.4.0.0"},
		},
		{
			name: "Vivaldi",
			ua:   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Vivaldi/6.7.3329.31",
			want: classifier.UAResult{Device: "desktop", Platform: "linux", OS: "linux", OSVersion: "", Browser: "vivaldi", BrowserVersion: "6.7.3329.31"},
		},
		{
			name: "UC Browser",
			ua:   "Mozilla/5.0 (Linux; U; Android 13; en-US; V2250) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0.4896.58 UCBrowser/13.4.0.1306 Mobile Safari/537.36",
			want: classifier.UAResult{Device: "mobile", Platform: "android", OS: "android", OSVersion: "13", Browser: "uc_browser", BrowserVersion: "13.4.0.1306"},
		},
		{
			name: "Huawei Browser",
			ua:   "Mozilla/5.0 (Linux; Android 12; ELS-NX9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.88 HuaweiBrowser/14.0.1.300 Mobile Safari/537.36",
			want: classifier.UAResult{Device: "mobile", Platform: "android", OS: "android", OSVersion: "12", Browser: "huawei_browser", BrowserVersion: "14.0.1.300"},
		},
		{
			name: "Mi Browser",
			ua:   "Mozilla/5.0 (Linux; U; Android 13; ru-ru; 2201116SG) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0.4896.127 Mobile Safari/537.36 XiaoMi/MiuiBrowser/14.2.0",
			want: classifier.UAResult{Device: "mobile", Platform: "android", OS: "android", OSVersion: "13", Browser: "mi_browser", BrowserVersion: "14.2.0"},
		},
		{
			name: "DuckDuckGo iOS",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 DuckDuckGo/7 Safari/605.1.15",
			want: classifier.UAResult{Device: "mobile", Platform: "ios", OS: "ios", OSVersion: "17.4", Browser: "duckduckgo", BrowserVersion: "7"},
		},
		{
			name: "Internet Explorer 11",
			ua:   "Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko",
			want: classifier.UAResult{Device: "desktop", Platform: "windows", OS: "windows", OSVersion: "10.0", Browser: "ie", BrowserVersion: "11.0"},
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
