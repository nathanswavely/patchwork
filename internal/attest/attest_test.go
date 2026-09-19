package attest_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/attest"
)

// keys mints a throwaway keypair through the same function the instance
// service actor uses, so the tests exercise the real key shapes.
func keys(t *testing.T) (pub, priv string) {
	t.Helper()
	pub, priv, err := ap.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return pub, priv
}

func signed(t *testing.T, priv, domain, nonce string, at time.Time) string {
	t.Helper()
	blob, err := attest.Sign(priv, attest.NewStatement(domain, nonce, at))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return blob
}

// The whole point: sign a verifier's nonce, hand it over, and have it check
// out against the public key served at the domain.
func TestSignThenVerifyRoundTrip(t *testing.T) {
	pub, priv := keys(t)
	now := time.Now().UTC()

	blob := signed(t, priv, "quilt.example.com", "host-ticket-40912", now)

	st, err := attest.Verify(blob, pub, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if st.Claim != attest.Claim {
		t.Errorf("claim = %q, want %q", st.Claim, attest.Claim)
	}
	if st.Domain != "quilt.example.com" {
		t.Errorf("domain = %q", st.Domain)
	}
	if st.Nonce != "host-ticket-40912" {
		t.Errorf("nonce = %q — the verifier's own string must come back verbatim", st.Nonce)
	}
	if st.KeyURL != "https://quilt.example.com/api/v1/instance/attestation-key" {
		t.Errorf("key_url = %q", st.KeyURL)
	}
}

// A blob is two base64url segments joined by a dot, and the signature
// covers the first segment's bytes. Pin the wire shape: a verifier writing
// against this format in another language has nothing else to go on.
func TestBlobShapeIsPayloadDotSignature(t *testing.T) {
	_, priv := keys(t)
	blob := signed(t, priv, "quilt.example.com", "nonce-for-shape", time.Now())

	parts := strings.Split(blob, ".")
	if len(parts) != 2 {
		t.Fatalf("blob has %d segments, want 2: %q", len(parts), blob)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("payload segment is not unpadded base64url: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	for _, field := range []string{"claim", "domain", "nonce", "issued_at", "expires_at", "key_url"} {
		if _, ok := got[field]; !ok {
			t.Errorf("payload is missing %q", field)
		}
	}
	if _, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
		t.Errorf("signature segment is not unpadded base64url: %v", err)
	}
}

// ADR 023 refuses to publish who the admins are, and this feature is only
// allowed to exist because it preserves that. The payload must not carry
// the signing admin's username, display name, email, or id — nor any field
// shaped like one.
func TestStatementNamesNobody(t *testing.T) {
	_, priv := keys(t)
	blob := signed(t, priv, "quilt.example.com", "no-names-please", time.Now())

	segment := strings.Split(blob, ".")[0]
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	allowed := map[string]bool{
		"claim": true, "domain": true, "nonce": true,
		"issued_at": true, "expires_at": true, "key_url": true,
	}
	for name := range fields {
		if !allowed[name] {
			t.Errorf("payload carries unexpected field %q — the statement is about the instance, not a person", name)
		}
	}
	for _, banned := range []string{"user", "username", "email", "admin_id", "display_name", "actor", "subject"} {
		if _, ok := fields[banned]; ok {
			t.Errorf("payload carries %q — ADR 023 keeps the roster unpublished", banned)
		}
	}
}

// Editing the statement — a longer expiry, somebody else's domain — has to
// break the signature, or the format proves nothing.
func TestTamperedPayloadFails(t *testing.T) {
	pub, priv := keys(t)
	now := time.Now().UTC()
	blob := signed(t, priv, "quilt.example.com", "tamper-me-please", now)

	parts := strings.Split(blob, ".")
	st, err := attest.Parse(blob)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	st.Domain = "attacker.example.com"
	st.KeyURL = attest.KeyURLFor(st.Domain)
	forged, _ := json.Marshal(st)
	tampered := base64.RawURLEncoding.EncodeToString(forged) + "." + parts[1]

	if _, err := attest.Verify(tampered, pub, now); err == nil {
		t.Fatal("a rewritten payload verified — the signature covers nothing")
	}
}

// A statement whose key_url points somewhere other than its own domain is
// the swap this format most invites: sign with your key, claim their
// domain, and name your own key endpoint. Refuse it even though the
// signature is genuine.
func TestKeyURLMustBelongToTheClaimedDomain(t *testing.T) {
	pub, priv := keys(t)
	now := time.Now().UTC()

	st := attest.NewStatement("victim.example.com", "swapped-key-url", now)
	st.KeyURL = "https://attacker.example.com/api/v1/instance/attestation-key"
	blob, err := attest.Sign(priv, st)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := attest.Verify(blob, pub, now); err == nil {
		t.Fatal("a statement pointing at a foreign key endpoint verified")
	}
}

// Fifteen minutes, then it is a receipt rather than a proof.
func TestExpiredFails(t *testing.T) {
	pub, priv := keys(t)
	issued := time.Now().UTC().Add(-2 * time.Hour)
	blob := signed(t, priv, "quilt.example.com", "long-ago-nonce", issued)

	_, err := attest.Verify(blob, pub, time.Now())
	if err == nil {
		t.Fatal("a two-hour-old attestation verified")
	}
	if !errors.Is(err, attest.ErrExpired) {
		t.Fatalf("error is %v, want ErrExpired — expired and forged must be distinguishable", err)
	}
}

// Still good a minute before the window closes.
func TestValidInsideTheWindow(t *testing.T) {
	pub, priv := keys(t)
	issued := time.Now().UTC()
	blob := signed(t, priv, "quilt.example.com", "still-warm-nonce", issued)

	if _, err := attest.Verify(blob, pub, issued.Add(attest.Lifetime-time.Minute)); err != nil {
		t.Fatalf("verify inside the window: %v", err)
	}
}

// Anybody can mint an RSA key and sign this JSON. What makes it a proof is
// that it checks against the key served at the domain.
func TestWrongKeyFails(t *testing.T) {
	_, priv := keys(t)
	otherPub, _ := keys(t)
	now := time.Now().UTC()

	blob := signed(t, priv, "quilt.example.com", "wrong-key-nonce", now)

	if _, err := attest.Verify(blob, otherPub, now); err == nil {
		t.Fatal("a statement verified against a key that never signed it")
	}
}

func TestMalformedBlobsAreRefused(t *testing.T) {
	pub, _ := keys(t)
	for _, blob := range []string{
		"",
		"notablob",
		"one.two.three",
		".",
		"!!!.!!!",
	} {
		if _, err := attest.Verify(blob, pub, time.Now()); err == nil {
			t.Errorf("blob %q verified", blob)
		}
	}
}

func TestValidateNonce(t *testing.T) {
	cases := []struct {
		name  string
		nonce string
		ok    bool
	}{
		{"ordinary", "host-ticket-40912", true},
		{"exactly the floor", "12345678", true},
		{"exactly the ceiling", strings.Repeat("a", 64), true},
		{"spaces are printable", "ticket 4091 please", true},
		{"unicode is printable", "quilt-vérification-01", true},
		{"too short", "abc", false},
		{"empty", "", false},
		{"too long", strings.Repeat("a", 65), false},
		{"newline", "abcdefgh\nijkl", false},
		{"carriage return", "abcdefgh\rijkl", false},
		{"tab", "abcdefgh\tijkl", false},
		{"null byte", "abcdefgh\x00ijkl", false},
		{"invalid utf-8", "abcdefgh\xff\xfe", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := attest.ValidateNonce(tc.nonce)
			if tc.ok && err != nil {
				t.Fatalf("ValidateNonce(%q) = %v, want ok", tc.nonce, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ValidateNonce(%q) was accepted", tc.nonce)
			}
		})
	}
}

// Parse reads a blob without judging it, which is what the CLI does before
// it knows which key to fetch. It must not be mistakable for verification.
func TestParseDoesNotVerify(t *testing.T) {
	_, priv := keys(t)
	blob := signed(t, priv, "quilt.example.com", "parse-only-nonce", time.Now())
	corrupted := strings.Split(blob, ".")[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("garbage"))

	st, err := attest.Parse(corrupted)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if st.Nonce != "parse-only-nonce" {
		t.Fatalf("nonce = %q", st.Nonce)
	}
}

// A clock a minute ahead on the signer must not make a fresh attestation
// look forged; a clock an hour ahead should.
func TestFutureDatingTolerance(t *testing.T) {
	pub, priv := keys(t)
	now := time.Now().UTC()

	nearFuture := signed(t, priv, "quilt.example.com", "skew-tolerant-01", now.Add(time.Minute))
	if _, err := attest.Verify(nearFuture, pub, now); err != nil {
		t.Fatalf("a minute of clock skew rejected the attestation: %v", err)
	}

	farFuture := signed(t, priv, "quilt.example.com", "skew-tolerant-02", now.Add(time.Hour))
	if _, err := attest.Verify(farFuture, pub, now); err == nil {
		t.Fatal("an attestation dated an hour ahead verified")
	}
}
