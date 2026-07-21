package ap_test

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
)

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}
	if pub == "" {
		t.Error("public key PEM is empty")
	}
	if priv == "" {
		t.Error("private key PEM is empty")
	}

	// Verify public key is parseable.
	pubBlock, _ := pem.Decode([]byte(pub))
	if pubBlock == nil {
		t.Fatal("failed to decode public key PEM")
	}
	if _, err := x509.ParsePKIXPublicKey(pubBlock.Bytes); err != nil {
		t.Fatalf("failed to parse public key: %v", err)
	}

	// Verify private key is parseable.
	privBlock, _ := pem.Decode([]byte(priv))
	if privBlock == nil {
		t.Fatal("failed to decode private key PEM")
	}
	if _, err := x509.ParsePKCS1PrivateKey(privBlock.Bytes); err != nil {
		t.Fatalf("failed to parse private key: %v", err)
	}
}

func TestSignAndVerify(t *testing.T) {
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com/ap/users/abc", nil)
	req.Header.Set("Host", "example.com")

	if err := ap.SignRequest(req, "https://example.com/ap/users/abc#main-key", priv); err != nil {
		t.Fatalf("SignRequest() error: %v", err)
	}

	if err := ap.VerifySignature(req, pub); err != nil {
		t.Errorf("VerifySignature() should succeed, got error: %v", err)
	}
}

func TestVerifyBadSignature(t *testing.T) {
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com/ap/users/abc", nil)
	req.Header.Set("Host", "example.com")

	if err := ap.SignRequest(req, "https://example.com/ap/users/abc#main-key", priv); err != nil {
		t.Fatalf("SignRequest() error: %v", err)
	}

	// Tamper with the Date header to invalidate the signature.
	req.Header.Set("Date", "Mon, 01 Jan 2001 00:00:00 GMT")

	if err := ap.VerifySignature(req, pub); err == nil {
		t.Error("VerifySignature() should fail after tampering, but got nil")
	}
}

func TestSignIncludesHostAndDetectsMismatch(t *testing.T) {
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	req := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader("test body"))
	if err := ap.SignRequest(req, "https://example.com/ap/users/abc#main-key", priv); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	// The signing string must cover the host pseudo-header so remote servers
	// (Mastodon et al.) that sign the real host can interoperate.
	if !strings.Contains(req.Header.Get("Signature"), "host") {
		t.Errorf("expected host in signed headers, got: %s", req.Header.Get("Signature"))
	}

	// Tampering with the host after signing must fail verification.
	req.Host = "evil.example"
	if err := ap.VerifySignature(req, pub); err == nil {
		t.Error("expected verification to fail after host tampering, got nil")
	}
}

func TestVerifyWrongKey(t *testing.T) {
	_, priv1, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}
	pub2, _, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com/ap/users/abc", nil)
	req.Header.Set("Host", "example.com")

	if err := ap.SignRequest(req, "https://example.com/ap/users/abc#main-key", priv1); err != nil {
		t.Fatalf("SignRequest() error: %v", err)
	}

	// Verify with a different key should fail.
	if err := ap.VerifySignature(req, pub2); err == nil {
		t.Error("VerifySignature() should fail with wrong key, but got nil")
	}
}

func TestSignSetsDateHeader(t *testing.T) {
	_, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com/ap/users/abc", nil)
	req.Header.Set("Host", "example.com")
	// Do not set Date header.

	if err := ap.SignRequest(req, "key-id", priv); err != nil {
		t.Fatalf("SignRequest() error: %v", err)
	}

	dateHeader := req.Header.Get("Date")
	if dateHeader == "" {
		t.Error("SignRequest() should set Date header when not present")
	}

	// Verify the date is parseable and recent.
	parsed, err := time.Parse(http.TimeFormat, dateHeader)
	if err != nil {
		t.Fatalf("Date header not in HTTP time format: %v", err)
	}
	if time.Since(parsed) > 5*time.Second {
		t.Errorf("Date header too old: %s", dateHeader)
	}
}

func TestSignPreservesExistingDate(t *testing.T) {
	_, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com/ap/users/abc", nil)
	req.Header.Set("Host", "example.com")

	existingDate := "Mon, 01 Jan 2024 12:00:00 GMT"
	req.Header.Set("Date", existingDate)

	if err := ap.SignRequest(req, "key-id", priv); err != nil {
		t.Fatalf("SignRequest() error: %v", err)
	}

	if got := req.Header.Get("Date"); got != existingDate {
		t.Errorf("SignRequest() changed Date header: got %s, want %s", got, existingDate)
	}
}
