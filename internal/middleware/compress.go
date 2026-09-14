package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Compress gzips responses that are worth gzipping.
//
// Patchwork serves its own SPA out of the binary, and nothing in the chain
// ever squeezed it: Caddy only reverse-proxies, and net/http's FileServer
// does not compress. Lancaster's instance was shipping 3 MB on a cold visit
// — a 1.4 MB script, a 1 MB map renderer, 270 KB of CSS — where the same
// bytes gzip to roughly a quarter of that. On the connections community
// organizers actually have, that difference is most of the wait before the
// first patch appears.
//
// Compressing here rather than in the Caddyfile is deliberate: an instance
// that runs the binary behind its own proxy, or behind none, is a supported
// deployment, and the wire should be small in all of them.

// Compression buys nothing on bytes that arrive compressed — woff2, png,
// webp, the zip an admin exports — and costs CPU on a Raspberry Pi. Decided
// by the Content-Type the handler declares, so the list is about kinds of
// payload rather than file extensions.
func worthCompressing(contentType string) bool {
	ct := contentType
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/json", ct == "application/javascript",
		ct == "application/manifest+json", ct == "application/xml",
		ct == "application/rss+xml", ct == "application/atom+xml",
		ct == "application/wasm", ct == "image/svg+xml",
		ct == "application/x-ndjson":
		return true
	// ActivityPub, WebFinger and JSON-LD travel as their own JSON flavors.
	case ct == "application/activity+json", ct == "application/jrd+json",
		ct == "application/ld+json":
		return true
	}
	return false
}

// Below this, the gzip header and trailer are a meaningful share of the
// response and the saving is noise. Only consulted when the handler declared
// a length; a streamed body of unknown size is compressed.
const minCompressSize = 1024

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true

	h := w.Header()
	// A handler that encoded its own body owns it; 204/304 carry none.
	if h.Get("Content-Encoding") != "" || status == http.StatusNoContent || status == http.StatusNotModified {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if !worthCompressing(h.Get("Content-Type")) {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n < minCompressSize {
		w.ResponseWriter.WriteHeader(status)
		return
	}

	// The compressed length is not the declared one, and ranges are offsets
	// into the identity body — neither survives, so both come off.
	h.Del("Content-Length")
	h.Del("Accept-Ranges")
	h.Set("Content-Encoding", "gzip")
	// An ETag is a tag for the bytes on the wire, and these are different
	// bytes. Weakening it keeps conditional requests working without
	// claiming the two encodings are byte-identical.
	if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		h.Set("ETag", "W/"+etag)
	}

	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz

	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		// An implicit 200: Go would sniff the type from these bytes, so
		// settle the header now that a Content-Type has had its chance.
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *gzipResponseWriter) close() {
	if w.gz == nil {
		return
	}
	w.gz.Close()
	w.gz.Reset(io.Discard) // don't pin the response writer in the pool
	gzipPool.Put(w.gz)
	w.gz = nil
}

// Compress wraps h so that compressible responses go out gzipped to clients
// that asked for it.
func Compress(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every response varies by this header whether or not this one was
		// compressed, or a shared cache will hand a gzipped body to a client
		// that cannot read it.
		w.Header().Add("Vary", "Accept-Encoding")

		if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			h.ServeHTTP(w, r)
			return
		}
		// A range request asks for offsets into the identity body; serving a
		// gzip stream instead answers a different question.
		if r.Header.Get("Range") != "" {
			h.ServeHTTP(w, r)
			return
		}

		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.close()
		h.ServeHTTP(gw, r)
	})
}

// acceptsGzip reads Accept-Encoding for gzip, honoring an explicit q=0 —
// which is how a client says "not this one".
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		name := strings.TrimSpace(strings.ToLower(fields[0]))
		if name != "gzip" && name != "*" {
			continue
		}
		for _, param := range fields[1:] {
			param = strings.TrimSpace(strings.ToLower(param))
			if q, ok := strings.CutPrefix(param, "q="); ok {
				if v, err := strconv.ParseFloat(q, 64); err == nil && v == 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}
