package handler

import (
	"net/url"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// An image is a reference, never bytes (docs/adr/007).
//
// One validator for patches and events, because two copies of a rule this
// small is how they end up disagreeing about what a URL is. The binary never
// fetches the image: the browser does, straight from wherever the patch keeps
// it, which is the whole point of the reference model and also the reason the
// checks here are about shape rather than content. There is nothing to
// inspect.

// maxImageURL is generous enough for a signed bucket URL, which is where these
// come from once docs/adr/007's upload flow lands.
const maxImageURL = 2048

// maxImageAlt is a caption, not a description of the image's contents. Long
// enough for a sentence about a flyer.
const maxImageAlt = 300

// validateImageRef checks a URL and its alt text together, because neither is
// valid without the other. Returns an empty string when the pair is fine.
//
// Clearing is always allowed: an empty URL with empty alt is a patch removing
// its image, and demanding alt text for a picture that no longer exists would
// trap somebody in a form.
func validateImageRef(rawURL, alt string) string {
	rawURL = strings.TrimSpace(rawURL)
	alt = strings.TrimSpace(alt)

	if rawURL == "" {
		return ""
	}
	if len(rawURL) > maxImageURL {
		return "that image address is too long"
	}
	if len(alt) > maxImageAlt {
		return "keep the description under 300 characters"
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "that doesn't look like an image address"
	}
	// https only. A patch page served over TLS that pulls an image over plain
	// http gets it blocked by the browser as mixed content, so an http URL is
	// a picture nobody will ever see — better refused at the form than
	// silently blank on every visitor's screen.
	if u.Scheme != "https" {
		return "the image address has to start with https://"
	}

	// Required alt text (docs/adr/007). It is the a11y baseline, and it is
	// what remains when the bytes go — these live on somebody else's host, so
	// they go more often than bytes we kept would.
	if alt == "" {
		return "add a short description of the image, so it still says something if it fails to load"
	}
	return ""
}

// checkPatchedImage validates an image edit arriving as a partial PATCH.
//
// The URL and its description are only valid as a pair, so a request carrying
// one of them has to be judged against what is already stored. Without that,
// sending only `image_url` reads as a picture with no description and gets
// refused, and sending only `image_alt` skips the check completely.
//
// `table` is interpolated, never taken from a request: the two callers pass
// literals, and the alternative was the same twenty lines written twice.
func checkPatchedImage(db *database.DB, table, id string, req map[string]interface{}) string {
	_, hasURL := req["image_url"]
	_, hasAlt := req["image_alt"]
	if !hasURL && !hasAlt {
		return ""
	}

	var curURL, curAlt string
	db.QueryRow(
		"SELECT COALESCE(image_url,''), COALESCE(image_alt,'') FROM "+table+" WHERE id = ?", id,
	).Scan(&curURL, &curAlt)

	nextURL, nextAlt := curURL, curAlt
	if v, ok := req["image_url"].(string); ok {
		nextURL = v
	}
	if v, ok := req["image_alt"].(string); ok {
		nextAlt = v
	}
	return validateImageRef(nextURL, nextAlt)
}

// maxEventURL matches maxImageURL: both are addresses somebody pasted, and
// ticket links carry query strings that get long.
const maxEventURL = 2048

// validateEventURL checks an event's own page out on the web (docs/adr/079).
//
// It lives beside the image validator because it is the same kind of rule —
// shape, not content; the binary never fetches either — but it is a separate
// function because the two disagree on one thing on purpose. An image over
// plain http is a picture nobody sees (mixed content); a *link* over plain
// http is a link that works, and half the venues in a small arts scene still
// serve one. Refusing those would mean refusing the very listings this field
// exists to point at.
//
// Empty is always fine: an event that has no page anywhere is the common case.
func validateEventURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) > maxEventURL {
		return "that link is too long"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "that doesn't look like a link — it should start with https://"
	}
	// http and https only. Anything else is either not a page a browser can
	// open or is a scheme (javascript:, data:) that has no business being
	// rendered as an href on somebody else's event.
	if u.Scheme != "https" && u.Scheme != "http" {
		return "that doesn't look like a link — it should start with https://"
	}
	return ""
}
