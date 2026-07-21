package governance

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/revlist"
)

// GitHTTPHandler returns an HTTP handler that serves git smart HTTP protocol
// for governance repos. This allows `git clone` and `git fetch` over HTTP.
//
// Routes:
//   GET  /api/v1/nodes/{slug}/governance.git/info/refs?service=git-upload-pack
//   POST /api/v1/nodes/{slug}/governance.git/git-upload-pack
//
// The caller must extract the nodeID from the URL and pass it.
func GitHTTPHandler(resolveNodeID func(slug string) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the slug and git path from the URL.
		// Expected URL patterns:
		//   /api/v1/nodes/{slug}/governance.git/info/refs
		//   /api/v1/nodes/{slug}/governance.git/git-upload-pack
		path := r.URL.Path

		// Find the governance.git part
		idx := strings.Index(path, "/governance.git/")
		if idx < 0 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		// Extract slug from the path before governance.git
		prefix := path[:idx]
		parts := strings.Split(strings.Trim(prefix, "/"), "/")
		if len(parts) < 1 {
			http.Error(w, "missing slug", http.StatusBadRequest)
			return
		}
		slug := parts[len(parts)-1]

		// Resolve slug to nodeID
		nodeID := resolveNodeID(slug)
		if nodeID == "" {
			http.Error(w, "patch not found", http.StatusNotFound)
			return
		}

		gitPath := path[idx+len("/governance.git"):]

		switch {
		case r.Method == "GET" && gitPath == "/info/refs":
			handleInfoRefs(w, r, nodeID)
		case r.Method == "POST" && gitPath == "/git-upload-pack":
			handleUploadPack(w, r, nodeID)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
}

// handleInfoRefs serves GET /info/refs?service=git-upload-pack
// Returns the list of refs (branches, tags) in the repo.
func handleInfoRefs(w http.ResponseWriter, r *http.Request, nodeID string) {
	service := r.URL.Query().Get("service")
	if service != "git-upload-pack" {
		http.Error(w, "only git-upload-pack is supported", http.StatusForbidden)
		return
	}

	dataDir := GetDataDir()
	if dataDir == "" {
		http.Error(w, "governance not configured", http.StatusInternalServerError)
		return
	}

	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	// Collect refs
	refs, err := repo.References()
	if err != nil {
		http.Error(w, "failed to list refs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
	w.Header().Set("Cache-Control", "no-cache")

	// Write pktline header
	writePktLine(w, "# service=git-upload-pack\n")
	writePktFlush(w)

	// Write refs in pktline format
	first := true
	refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.SymbolicReference {
			return nil // skip HEAD symbolic ref in listing
		}
		hash := ref.Hash().String()
		name := ref.Name().String()
		if first {
			// First line includes capabilities
			fmt.Fprintf(w, "%04x%s %s\x00side-band-64k ofs-delta\n", len(hash)+1+len(name)+1+len("side-band-64k ofs-delta")+5, hash, name)
			first = false
		} else {
			writePktLine(w, fmt.Sprintf("%s %s\n", hash, name))
		}
		return nil
	})

	// If empty repo (no refs), still need to send flush
	writePktFlush(w)
}

// handleUploadPack serves POST /git-upload-pack
// Receives a list of wants/haves and returns a packfile.
func handleUploadPack(w http.ResponseWriter, r *http.Request, nodeID string) {
	dataDir := GetDataDir()
	if dataDir == "" {
		http.Error(w, "governance not configured", http.StatusInternalServerError)
		return
	}

	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}

	// Read the request body (may be gzip-encoded)
	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "bad gzip", http.StatusBadRequest)
			return
		}
		defer gz.Close()
		body = gz
	}

	// Parse upload-pack request
	upr := packp.NewUploadPackRequest()
	if err := upr.Decode(body); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	// Build the packfile
	storer := repo.Storer

	// Collect the objects needed
	var haves []plumbing.Hash
	for _, h := range upr.Haves {
		haves = append(haves, h)
	}

	var wants []plumbing.Hash
	for _, w := range upr.Wants {
		wants = append(wants, w)
	}

	if len(wants) == 0 {
		// Nothing wanted — send empty response
		writePktLine(w, "NAK\n")
		writePktFlush(w)
		return
	}

	// Get the objects to send
	objects, err := revlist.Objects(storer, wants, haves)
	if err != nil {
		http.Error(w, fmt.Sprintf("revlist: %v", err), http.StatusInternalServerError)
		return
	}

	// Write NAK (we don't do multi_ack)
	io.WriteString(w, "0008NAK\n")

	// Encode packfile
	var packBuf bytes.Buffer
	enc := packfile.NewEncoder(&packBuf, storer, false)

	_, err = enc.Encode(objects, 0)
	if err != nil {
		// Response already started, can't send HTTP error
		return
	}

	w.Write(packBuf.Bytes())
}

// writePktLine writes a pkt-line formatted string.
func writePktLine(w io.Writer, data string) {
	fmt.Fprintf(w, "%04x%s", len(data)+4, data)
}

// writePktFlush writes a pkt-line flush packet.
func writePktFlush(w io.Writer) {
	fmt.Fprint(w, "0000")
}
