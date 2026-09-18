package ap_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
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
	// A POST has to carry a Digest for its signature to cover one, the way
	// delivery.go sets it on every outbound activity. Without it this request
	// would be turned away for the missing digest before the host it is
	// actually testing was ever compared.
	sum := sha256.Sum256([]byte("test body"))
	req.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
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


// A signature is only worth as much as the list of headers it covers, so the
// verifier names that list rather than accepting the sender's (docs/adr/113).

// signedWithHeaders signs req by hand over exactly the headers named, which is
// what a sender that covers less than our own SignRequest does looks like on
// the wire.
func signedWithHeaders(t *testing.T, req *http.Request, keyID, privPEM string, headers []string) {
	t.Helper()
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		t.Fatal("decode private key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}

	var parts []string
	for _, h := range headers {
		switch h {
		case "(request-target)":
			parts = append(parts, fmt.Sprintf("(request-target): %s %s", strings.ToLower(req.Method), req.URL.RequestURI()))
		case "host":
			host := req.Host
			if host == "" {
				host = req.URL.Host
			}
			parts = append(parts, "host: "+host)
		default:
			parts = append(parts, fmt.Sprintf("%s: %s", h, req.Header.Get(h)))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	req.Header.Set("Signature", fmt.Sprintf(`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		keyID, strings.Join(headers, " "), base64.StdEncoding.EncodeToString(sig)))
}

func TestVerifyRejectsASignatureThatCoversTooLittle(t *testing.T) {
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	keyID := "https://remote.example/ap/users/abc#main-key"

	cases := []struct {
		name    string
		method  string
		headers []string
		missing string
	}{
		{"date alone, the spec default", "POST", []string{"date"}, "(request-target)"},
		{"no request target", "POST", []string{"host", "date", "digest"}, "(request-target)"},
		{"no host", "POST", []string{"(request-target)", "date", "digest"}, "host"},
		{"no date, which the skew check assumes is signed", "POST", []string{"(request-target)", "host", "digest"}, "date"},
		{"no digest, which leaves the body unsigned", "POST", []string{"(request-target)", "host", "date"}, "digest"},
		{"a GET missing its host", "GET", []string{"(request-target)", "date"}, "host"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "https://example.com/inbox", strings.NewReader("test body"))
		sum := sha256.Sum256([]byte("test body"))
		req.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
		signedWithHeaders(t, req, keyID, priv, tc.headers)

		err := ap.VerifySignature(req, pub)
		if err == nil {
			t.Errorf("%s: expected rejection, got nil", tc.name)
			continue
		}
		// The signature itself is cryptographically sound over what it covers,
		// so the refusal has to be about coverage and not about arithmetic.
		if !strings.Contains(err.Error(), tc.missing) {
			t.Errorf("%s: expected the error to name %q, got %v", tc.name, tc.missing, err)
		}
	}
}

func TestVerifyAcceptsTheRequiredSetAndMore(t *testing.T) {
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	keyID := "https://remote.example/ap/users/abc#main-key"

	post := httptest.NewRequest("POST", "https://example.com/inbox", strings.NewReader("test body"))
	sum := sha256.Sum256([]byte("test body"))
	post.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
	post.Header.Set("Content-Type", "application/activity+json")
	// More than the minimum is fine: the rule is a floor, not a shape.
	signedWithHeaders(t, post, keyID, priv, []string{"(request-target)", "host", "date", "digest", "content-type"})
	if err := ap.VerifySignature(post, pub); err != nil {
		t.Errorf("a POST covering the required set and one extra was rejected: %v", err)
	}

	get, _ := http.NewRequest("GET", "https://example.com/ap/users/abc", nil)
	get.Host = "example.com"
	signedWithHeaders(t, get, keyID, priv, []string{"(request-target)", "host", "date"})
	if err := ap.VerifySignature(get, pub); err != nil {
		t.Errorf("a GET covering the required set was rejected: %v", err)
	}
}
