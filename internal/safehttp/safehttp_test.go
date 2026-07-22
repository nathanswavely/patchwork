package safehttp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// A guarded client must refuse non-public addresses at dial time, and the
// test seam must lift the guard.
func TestNewClient_RefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := safehttp.NewClient(5 * time.Second)

	// srv listens on 127.0.0.1 — the guard must refuse it.
	_, err := client.Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "ssrf guard") {
		t.Fatalf("expected ssrf guard refusal for loopback fetch, got %v", err)
	}

	prev := safehttp.SetAllowPrivateAddresses(true)
	defer safehttp.SetAllowPrivateAddresses(prev)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("fetch with guard lifted: %v", err)
	}
	resp.Body.Close()
}
