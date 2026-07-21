package ap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
)

// The default actor fetcher must refuse non-public addresses: actor URLs
// are attacker-influenced (inbound keyIds, user-supplied remote follows),
// so fetching them must never probe the host's own network.
func TestFetchActor_RefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"x","inbox":"x/inbox","publicKey":{"publicKeyPem":"pem"}}`))
	}))
	defer srv.Close()
	ap.ClearActorCache()
	t.Cleanup(ap.ClearActorCache)

	// srv listens on 127.0.0.1 — the guard must refuse it.
	_, err := ap.FetchActor(context.Background(), srv.URL+"/actor")
	if err == nil || !strings.Contains(err.Error(), "ssrf guard") {
		t.Fatalf("expected ssrf guard refusal for loopback fetch, got %v", err)
	}

	// The seam (for tests and loopback dev federation) lifts the guard.
	prev := ap.SetAllowPrivateAddresses(true)
	defer ap.SetAllowPrivateAddresses(prev)
	actor, err := ap.FetchActor(context.Background(), srv.URL+"/actor")
	if err != nil {
		t.Fatalf("fetch with guard lifted: %v", err)
	}
	if actor.PublicKey != "pem" {
		t.Errorf("unexpected actor: %+v", actor)
	}
}

func TestFetchActor_RejectsNonHTTPSchemes(t *testing.T) {
	ap.ClearActorCache()
	t.Cleanup(ap.ClearActorCache)
	_, err := ap.FetchActor(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error for non-http scheme")
	}
}
