package auth

import "strings"

// DeviceLabel turns a raw User-Agent string into a coarse, human-readable
// label like "Chrome on Windows" or "Safari on iPhone", for the session
// manager (issue #3). It is deliberately not a forensic UA parser: a person
// only needs enough to recognize which of their own sessions is which, so a
// tiny hand-rolled matcher over the major browser and OS families is all this
// is — no dependency, no version numbers, no device fingerprinting.
//
// An empty or unrecognizable agent (pre-migration rows, exotic clients) reads
// as "Unknown device" rather than leaking the raw string into the UI.
func DeviceLabel(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return "Unknown device"
	}

	browser := browserFamily(ua)
	os := osFamily(ua)

	switch {
	case browser != "" && os != "":
		return browser + " on " + os
	case browser != "":
		return browser
	case os != "":
		return os
	default:
		return "Unknown device"
	}
}

// browserFamily picks the major browser. Order matters: Edge and Opera both
// carry "Chrome" in their UA, and Chrome carries "Safari", so the more
// specific tokens are tested first.
func browserFamily(ua string) string {
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "Edge/"):
		return "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		return "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS"):
		return "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS"):
		return "Chrome"
	case strings.Contains(ua, "Safari/"):
		return "Safari"
	default:
		return ""
	}
}

// osFamily picks the major operating system. iPhone/iPad are tested before the
// generic "Mac OS X" token that iOS UAs also carry, and Android before Linux.
func osFamily(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		return "macOS"
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	default:
		return ""
	}
}
