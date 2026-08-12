package classifier

import (
	"regexp"
	"strings"
)

// UAResult buckets a User-Agent string into the same small, fixed
// vocabulary the frontend's FIELD_VOCAB (lib/filters.ts) and the routing
// engine's conformance fixture already assume. Platform/OS/Browser leave
// "" (unrecognized) rather than guess when no known marker matches — an
// empty classified value simply never matches an IS/IN filter, which is
// safe; inventing a wrong-but-confident bucket would not be. Device is the
// one exception: it defaults to "desktop" absent any mobile/tablet marker,
// the same convention real UA parsers use, since "not mobile, not tablet"
// is itself a meaningful, safe-to-assume signal rather than a guess.
type UAResult struct {
	Device         string // "mobile" | "tablet" | "desktop"
	Platform       string // "ios" | "android" | "windows" | "macos" | "linux"
	OS             string // same vocabulary/value as Platform — the frontend treats them as one vocabulary
	OSVersion      string
	Browser        string // "chrome" | "safari" | "firefox" | "edge" | "samsung_internet" | "other"
	BrowserVersion string
}

var (
	reIOSVersion     = regexp.MustCompile(`(?:CPU (?:iPhone )?OS|iPhone OS) (\d+)[_.](\d+)`)
	reAndroidVersion = regexp.MustCompile(`Android (\d+(?:\.\d+)?)`)
	reWindowsVersion = regexp.MustCompile(`Windows NT ([\d.]+)`)
	reMacVersion     = regexp.MustCompile(`Mac OS X (\d+[_.]\d+(?:[_.]\d+)?)`)

	reEdgeVersion    = regexp.MustCompile(`Edg(?:A|iOS)?/([\d.]+)`)
	reSamsungVersion = regexp.MustCompile(`SamsungBrowser/([\d.]+)`)
	reChromeVersion  = regexp.MustCompile(`Chrome/([\d.]+)`)
	reFirefoxVersion = regexp.MustCompile(`Firefox/([\d.]+)`)
	reSafariVersion  = regexp.MustCompile(`Version/([\d.]+)`) // Safari doesn't version itself in "Safari/", it uses "Version/"
)

func ParseUserAgent(ua string) UAResult {
	return UAResult{
		Device:         classifyDevice(ua),
		Platform:       classifyPlatformOS(ua),
		OS:             classifyPlatformOS(ua),
		OSVersion:      classifyOSVersion(ua),
		Browser:        classifyBrowser(ua),
		BrowserVersion: classifyBrowserVersion(ua),
	}
}

func classifyDevice(ua string) string {
	switch {
	case strings.Contains(ua, "iPad"):
		return "tablet"
	case strings.Contains(ua, "Android") && !strings.Contains(ua, "Mobile"):
		return "tablet"
	case strings.Contains(ua, "Tablet"):
		return "tablet"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "Mobile"):
		return "mobile"
	case ua == "":
		return ""
	default:
		return "desktop"
	}
}

func classifyPlatformOS(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iPod"):
		return "ios"
	case strings.Contains(ua, "Android"):
		return "android"
	case strings.Contains(ua, "Windows"):
		return "windows"
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		return "macos"
	case strings.Contains(ua, "Linux"):
		return "linux"
	default:
		return ""
	}
}

func classifyOSVersion(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iPod"):
		if m := reIOSVersion.FindStringSubmatch(ua); m != nil {
			return m[1] + "." + m[2]
		}
	case strings.Contains(ua, "Android"):
		if m := reAndroidVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	case strings.Contains(ua, "Windows"):
		if m := reWindowsVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		if m := reMacVersion.FindStringSubmatch(ua); m != nil {
			return strings.ReplaceAll(m[1], "_", ".")
		}
	}
	return ""
}

// classifyBrowser checks Edge and Samsung Internet before Chrome, and
// Chrome before Safari, because all three embed "Chrome/" and/or
// "Safari/" tokens in their own User-Agent strings (they're Chromium- or
// WebKit-based) — order is the whole trick here.
func classifyBrowser(ua string) string {
	switch {
	case strings.Contains(ua, "Edg/"), strings.Contains(ua, "EdgA/"), strings.Contains(ua, "EdgiOS/"):
		return "edge"
	case strings.Contains(ua, "SamsungBrowser/"):
		return "samsung_internet"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		return "chrome"
	case strings.Contains(ua, "Firefox/"), strings.Contains(ua, "FxiOS/"):
		return "firefox"
	case strings.Contains(ua, "Safari/"):
		return "safari"
	case ua == "":
		return ""
	default:
		return "other"
	}
}

func classifyBrowserVersion(ua string) string {
	switch classifyBrowser(ua) {
	case "edge":
		if m := reEdgeVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	case "samsung_internet":
		if m := reSamsungVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	case "chrome":
		if m := reChromeVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	case "firefox":
		if m := reFirefoxVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	case "safari":
		if m := reSafariVersion.FindStringSubmatch(ua); m != nil {
			return m[1]
		}
	}
	return ""
}
