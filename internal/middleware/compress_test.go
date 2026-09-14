package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveCompressed(t *testing.T, acceptEncoding string, h http.HandlerFunc) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	Compress(h).ServeHTTP(rec, req)
	return rec.Result()
}

func decodedBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var r io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer gz.Close()
		r = gz
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// The whole point: the SPA bundle is the biggest thing Patchwork sends, and
// it was going out raw.
func TestCompressesScript(t *testing.T) {
	payload := strings.Repeat("const patchwork = 'be the pattern';\n", 200)
	resp := serveCompressed(t, "gzip, deflate, br", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		io.WriteString(w, payload)
	})

	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := decodedBody(t, resp); got != payload {
		t.Error("body did not round-trip through gzip")
	}
}

// Handlers write JSON with an encoder, so the length is unknown when the
// header goes out. That path has to compress too — nodes/tree is 100 KB.
func TestCompressesStreamedJSON(t *testing.T) {
	resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"tree":[`+strings.Repeat(`{"slug":"a-patch"},`, 200)+`null]}`)
	})
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := resp.Header.Get("Content-Length"); got != "" {
		t.Errorf("Content-Length survived compression: %q", got)
	}
}

// Fonts, avatars and an admin's export zip arrive compressed already;
// re-compressing them spends a Raspberry Pi's CPU to make them bigger.
func TestLeavesCompressedTypesAlone(t *testing.T) {
	for _, ct := range []string{"font/woff2", "image/png", "image/webp", "application/zip", "video/mp4"} {
		resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			w.Write(bytes.Repeat([]byte{0x7f}, 4096))
		})
		if got := resp.Header.Get("Content-Encoding"); got != "" {
			t.Errorf("%s: Content-Encoding = %q, want none", ct, got)
		}
	}
}

// SVG is the exception in that family — it is markup, and the instance icon
// is served as one.
func TestCompressesSVG(t *testing.T) {
	resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		io.WriteString(w, strings.Repeat(`<rect width="8" height="8"/>`, 100))
	})
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
}

// Federation and feeds carry their own JSON and XML flavors.
func TestCompressesFederationTypes(t *testing.T) {
	for _, ct := range []string{
		"application/activity+json",
		"application/jrd+json",
		"application/ld+json",
		"application/rss+xml",
		"text/calendar; charset=utf-8",
	} {
		resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			io.WriteString(w, strings.Repeat("a-patch,", 500))
		})
		if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
			t.Errorf("%s: Content-Encoding = %q, want gzip", ct, got)
		}
	}
}

func TestSkipsWhenClientDoesNotAsk(t *testing.T) {
	for _, ae := range []string{"", "br", "gzip;q=0", "identity"} {
		resp := serveCompressed(t, ae, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, strings.Repeat("<p>hello</p>", 200))
		})
		if got := resp.Header.Get("Content-Encoding"); got != "" {
			t.Errorf("Accept-Encoding %q: Content-Encoding = %q, want none", ae, got)
		}
	}
}

// A shared cache that does not key on this header will hand a gzipped body
// to a client that cannot read it — so it goes out whether or not this
// particular response was compressed.
func TestAlwaysVaries(t *testing.T) {
	for _, ae := range []string{"", "gzip"} {
		resp := serveCompressed(t, ae, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, strings.Repeat("<p>hello</p>", 200))
		})
		if got := resp.Header.Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
			t.Errorf("Accept-Encoding %q: Vary = %q", ae, got)
		}
	}
}

// The gzip header and trailer are most of a short response.
func TestSkipsTinyDeclaredBodies(t *testing.T) {
	resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "22")
		io.WriteString(w, `{"ok":true,"n":123456}`)
	})
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
}

// A range is an offset into the identity body; a gzip stream answers a
// different question than the one asked.
func TestSkipsRangeRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-99")
	rec := httptest.NewRecorder()
	Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, strings.Repeat("x", 4096))
	})).ServeHTTP(rec, req)

	if got := rec.Result().Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
}

// A handler that encoded its own body owns it.
func TestDoesNotDoubleEncode(t *testing.T) {
	resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "br")
		w.Write(bytes.Repeat([]byte{0x11}, 4096))
	})
	if got := resp.Header.Get("Content-Encoding"); got != "br" {
		t.Errorf("Content-Encoding = %q, want br", got)
	}
}

func TestLeavesBodilessResponsesAlone(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
		})
		if got := resp.Header.Get("Content-Encoding"); got != "" {
			t.Errorf("%d: Content-Encoding = %q, want none", status, got)
		}
	}
}

// Same URL, two different sets of bytes on the wire — a strong tag would
// claim they are interchangeable.
func TestWeakensETagOnCompressedBodies(t *testing.T) {
	resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("ETag", `"abc123"`)
		io.WriteString(w, strings.Repeat(".patch{color:red}", 200))
	})
	if got, want := resp.Header.Get("ETag"), `W/"abc123"`; got != want {
		t.Errorf("ETag = %q, want %q", got, want)
	}
}

// The status a handler chose has to reach the client unchanged.
func TestPreservesStatus(t *testing.T) {
	resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, strings.Repeat("<p>gone</p>", 200))
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
}

// Pooled writers are reset and reused; a leaked reference would send one
// response's bytes into another's connection.
func TestPooledWritersStayIndependent(t *testing.T) {
	for i := range 50 {
		want := strings.Repeat("patch-", 300) + string(rune('a'+i%26))
		resp := serveCompressed(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, want)
		})
		if got := decodedBody(t, resp); got != want {
			t.Fatalf("iteration %d: body mismatch", i)
		}
	}
}
