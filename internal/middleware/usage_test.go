package middleware

import (
	"net/http/httptest"
	"testing"
)

// The counter's privacy properties are what these tests pin, more than its
// arithmetic (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).

func TestNormalizeUsagePathKeepsShapeNotIdentity(t *testing.T) {
	cases := map[string]string{
		"/":                           "/",
		"":                            "/",
		"/patches":                    "/patches",
		"/patches/new":                "/patches/new",
		"/patches/the-selvage":        "/patches/{slug}",
		"/patches/the-selvage/events": "/patches/{slug}/events",
		"/patches/the-selvage/governance/docs/0199a1b2-1234-7abc-8def-0123456789ab": "/patches/{slug}/governance/docs",
		"/patches/the-selvage/events/0199a1b2-1234-7abc-8def-0123456789ab":          "/patches/{slug}/events/{id}",
		"/users/nathan": "/users/{username}",
		"/events/0199a1b2-1234-7abc-8def-0123456789ab":      "/events/{id}",
		"/events/0199a1b2-1234-7abc-8def-0123456789ab/edit": "/events/{id}/edit",
		"/events/my":           "/events/my",
		"/invite/s3cr3t-token": "/invite/{token}",
		"/quilts/arts.example/patches/gallery-row": "/quilts/{host}/patches/{slug}",
		"/map/my":                    "/map/my",
		"/admin/usage":               "/admin/usage",
		"/wp-admin/setup-config.php": "(other)",
		"/.env":                      "(other)",
	}
	for in, want := range cases {
		if got := NormalizeUsagePath(in); got != want {
			t.Errorf("NormalizeUsagePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountableDocumentSkipsWhatIsNotAPersonLoadingAPage(t *testing.T) {
	req := func(path, ua, dest string) bool {
		r := httptest.NewRequest("GET", path, nil)
		if ua != "" {
			r.Header.Set("User-Agent", ua)
		}
		if dest != "" {
			r.Header.Set("Sec-Fetch-Dest", dest)
		}
		return countableDocument(r)
	}
	browser := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36"

	if !req("/patches/the-selvage", browser, "document") {
		t.Error("a browser navigating to a page is a page load")
	}
	if !req("/", browser, "") {
		t.Error("a browser that sends no Sec-Fetch-Dest still counts")
	}
	if req("/assets/index-abc123.js", browser, "script") {
		t.Error("an asset is not a page load")
	}
	if req("/favicon.svg", browser, "image") {
		t.Error("a file is not a page load")
	}
	if req("/api/v1/nodes", browser, "empty") {
		t.Error("an API call is not a page load")
	}
	if req("/patches/the-selvage", browser, "empty") {
		t.Error("a fetch the browser labels as not a navigation is not a page load")
	}
	if req("/", "", "") {
		t.Error("no user agent means software, not a person")
	}
	for _, ua := range []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"facebookexternalhit/1.1",
		"http.rb/5.1.1 (Mastodon/4.2.0; +https://mastodon.social/)",
		"curl/8.4.0",
		"python-requests/2.31",
	} {
		if req("/", ua, "") {
			t.Errorf("%q should not be counted", ua)
		}
	}
}

func TestRecordCountsAVisitorOnceADayAndKeepsNoAddress(t *testing.T) {
	c := NewUsageCounter(nil, func() bool { return true })

	c.Record("203.0.113.7", "Mozilla/5.0", "/")
	c.Record("203.0.113.7", "Mozilla/5.0", "/patches/{slug}")
	c.Record("203.0.113.9", "Mozilla/5.0", "/")

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.views["/"] != 2 || c.views["/patches/{slug}"] != 1 {
		t.Errorf("views = %v", c.views)
	}
	if len(c.visitors) != 2 {
		t.Errorf("two browsers should be two visitors, got %d", len(c.visitors))
	}
	for k := range c.visitors {
		if string(k[:]) == "203.0.113.7" {
			t.Error("the visitor set must hold hashes, never addresses")
		}
	}
}

func TestRotateReplacesTheSaltSoDaysCannotBeJoined(t *testing.T) {
	c := NewUsageCounter(nil, func() bool { return true })
	c.Record("203.0.113.7", "Mozilla/5.0", "/")
	c.mu.Lock()
	var first [16]byte
	for k := range c.visitors {
		first = k
	}
	oldSalt := c.salt
	c.rotate("2999-01-01")
	c.mu.Unlock()

	if c.salt == oldSalt {
		t.Fatal("a new day must draw a new salt")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.visitors) != 0 || len(c.views) != 0 {
		t.Error("a new day starts empty")
	}
	if c.pendingV["2999-01-01"] != 0 {
		t.Error("nothing pending for the new day")
	}
	// Same browser, next day: a different hash, because the salt moved.
	c.mu.Unlock()
	c.Record("203.0.113.7", "Mozilla/5.0", "/")
	c.mu.Lock()
	for k := range c.visitors {
		if k == first {
			t.Error("the same browser must hash differently on a different day")
		}
	}
}

func TestRotateCarriesUnwrittenCountsToTheNextFlush(t *testing.T) {
	c := NewUsageCounter(nil, func() bool { return true })
	c.Record("203.0.113.7", "Mozilla/5.0", "/")
	c.Record("203.0.113.8", "Mozilla/5.0", "/")
	c.mu.Lock()
	yesterday := c.day
	c.rotate("2999-01-01")
	c.mu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[yesterday]["/"] != 2 {
		t.Errorf("yesterday's views should be pending, got %v", c.pending)
	}
	if c.pendingV[yesterday] != 2 {
		t.Errorf("yesterday's visitors should be pending, got %v", c.pendingV)
	}
}
