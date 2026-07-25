package ap

// Internal test: deliverActivity is unexported, and the point here is to
// verify the real wiring — the delivery client must refuse non-public
// inbox addresses. Inbox URLs come from remote actor documents, which are
// attacker-influenced.

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func TestDeliverActivity_RefusesPrivateAddresses(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "patchwork-ap-delivery-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(tmpFile.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// A local actor with a keypair, so signing succeeds and the request
	// reaches the dialer where the guard lives.
	userAPID := "https://test.example.com/ap/users/user-deliver-1"
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role, ap_id, created_at, updated_at)
		 VALUES ('user-deliver-1', 'd@example.com', 'deliverer', 'Deliverer', 'member', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		userAPID,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, _, err := EnsureUserKeypair(db, "user-deliver-1"); err != nil {
		t.Fatalf("ensure keypair: %v", err)
	}

	activity, _ := json.Marshal(map[string]any{"type": "Create", "actor": userAPID})

	var served bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	// srv listens on 127.0.0.1 — the guard must refuse it before connecting.
	err = deliverActivity(context.Background(), db, string(activity), srv.URL+"/inbox")
	if err == nil || !strings.Contains(err.Error(), "ssrf guard") {
		t.Fatalf("expected ssrf guard refusal for loopback inbox, got %v", err)
	}
	if served {
		t.Fatal("request reached the server despite the guard")
	}

	// The seam (for tests and loopback dev federation) lifts the guard.
	prev := SetAllowPrivateAddresses(true)
	defer SetAllowPrivateAddresses(prev)
	if err := deliverActivity(context.Background(), db, string(activity), srv.URL+"/inbox"); err != nil {
		t.Fatalf("deliver with guard lifted: %v", err)
	}
	if !served {
		t.Fatal("request never reached the server with guard lifted")
	}
}
