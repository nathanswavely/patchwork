// Package attest issues and checks the short-lived statement an instance
// admin hands an outside party to prove they administer a quilt
// (docs/adr/087).
//
// The statement says one thing: an admin of this domain signed this nonce
// at this time. It never names the admin — no username, no email, no user
// id — because publishing who holds the admin bit is exactly what ADR 023
// refused. The verifier learns that somebody with the keys answered, which
// is the whole question a hosting provider or a directory is asking.
//
// The format is a detached-JWS-style compact serialization:
//
//	base64url(payload JSON) "." base64url(signature)
//
// Both segments are unpadded base64url (RFC 4648 §5). The signature is
// RSASSA-PKCS1-v1_5 over SHA-256 — "RS256" — computed over the ASCII bytes
// of the *first segment*, not over the decoded JSON. Signing the encoded
// text is what keeps verification free of any JSON canonicalization
// question: the exact bytes that were signed are sitting in the blob.
//
// The key is the instance service actor's (internal/ap), the same RSA
// keypair that signs ActivityPub deliveries. One instance identity, one
// key, whether or not the instance federates.
package attest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// Claim is the only claim this format carries. A statement asserting
	// anything else is not one of ours.
	Claim = "instance-admin"

	// Algorithm names the signature scheme in the register a verifier's
	// tooling uses. It is fixed by the format and never read back out of a
	// statement — a verifier that takes its algorithm from the thing it is
	// verifying is the oldest hole in signed-token design.
	Algorithm = "RS256"

	// KeyPath is where every instance serves the public half, federation on
	// or off.
	KeyPath = "/api/v1/instance/attestation-key"

	// Lifetime is how long a statement stands. Long enough to paste into a
	// support ticket, short enough that a leaked blob is worthless by the
	// time anyone finds it.
	Lifetime = 15 * time.Minute

	// MinNonceLen and MaxNonceLen bound the verifier's string. The floor is
	// not a security control — the nonce's unguessability is the verifier's
	// business, not ours — it just refuses a nonce so short it was probably
	// a typo.
	MinNonceLen = 8
	MaxNonceLen = 64

	// clockSkew is how far into the future an issued_at may sit before a
	// verifier calls it wrong. Two machines, two clocks.
	clockSkew = 2 * time.Minute
)

// ErrExpired is returned by Verify for a signature that checks out over a
// statement whose window has closed. It is distinguished from a bad
// signature on purpose: "this was genuine an hour ago" and "this was never
// genuine" are different answers to give somebody.
var ErrExpired = errors.New("attestation has expired")

// Statement is the signed payload. Every field is about the instance; none
// is about a person.
type Statement struct {
	Claim     string `json:"claim"`
	Domain    string `json:"domain"`
	Nonce     string `json:"nonce"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
	// KeyURL is where the verifier fetches the public half. It is inside
	// the signature and pinned to Domain, so it cannot be swung at a key
	// the signer controls — see Verify.
	KeyURL string `json:"key_url"`
}

// KeyURLFor returns the canonical key endpoint for a domain.
func KeyURLFor(domain string) string {
	return "https://" + domain + KeyPath
}

// NewStatement builds the statement for a nonce, issued now.
func NewStatement(domain, nonce string, now time.Time) Statement {
	now = now.UTC().Truncate(time.Second)
	return Statement{
		Claim:     Claim,
		Domain:    domain,
		Nonce:     nonce,
		IssuedAt:  now.Format(time.RFC3339),
		ExpiresAt: now.Add(Lifetime).Format(time.RFC3339),
		KeyURL:    KeyURLFor(domain),
	}
}

// ValidateNonce checks the verifier-supplied string before it is signed and
// audited. It has to survive a JSON round trip, a copy-paste, and a line in
// the audit log, so: printable, one line, bounded.
func ValidateNonce(nonce string) error {
	if !utf8.ValidString(nonce) {
		return errors.New("nonce must be valid UTF-8")
	}
	n := utf8.RuneCountInString(nonce)
	if n < MinNonceLen {
		return fmt.Errorf("nonce must be at least %d characters", MinNonceLen)
	}
	if n > MaxNonceLen {
		return fmt.Errorf("nonce must be at most %d characters", MaxNonceLen)
	}
	for _, r := range nonce {
		// IsPrint is false for every control character, newlines included,
		// so a nonce cannot smuggle a second line into the audit record or
		// break the blob across two lines in a mail client.
		if !unicode.IsPrint(r) {
			return errors.New("nonce must be printable text on one line")
		}
	}
	return nil
}

// Sign encodes a statement and signs it with a PEM-encoded RSA private key
// — the instance service actor's, in production.
func Sign(privateKeyPEM string, st Statement) (string, error) {
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(st)
	if err != nil {
		return "", fmt.Errorf("encode statement: %w", err)
	}
	segment := b64.EncodeToString(payload)

	sum := sha256.Sum256([]byte(segment))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign statement: %w", err)
	}
	return segment + "." + b64.EncodeToString(sig), nil
}

// Parse decodes a blob without checking its signature. It answers "what
// does this claim to say" and nothing else — never use it to decide
// anything.
func Parse(blob string) (Statement, error) {
	segment, _, err := split(blob)
	if err != nil {
		return Statement{}, err
	}
	return decodePayload(segment)
}

// Verify checks a blob against a public key and returns the statement it
// carries. The key must be the one served at the domain the caller cares
// about — never the one named by the blob it is checking.
//
// It fails a statement whose key_url does not point at its own domain.
// Without that check the format would invite the mistake it is most likely
// to invite: fetch the key the blob names, verify against it, and conclude
// that whoever signed it runs a domain they have never touched.
func Verify(blob, publicKeyPEM string, now time.Time) (Statement, error) {
	segment, sig, err := split(blob)
	if err != nil {
		return Statement{}, err
	}

	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return Statement{}, err
	}

	sum := sha256.Sum256([]byte(segment))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return Statement{}, errors.New("signature does not verify against this key")
	}

	st, err := decodePayload(segment)
	if err != nil {
		return Statement{}, err
	}
	if st.Claim != Claim {
		return st, fmt.Errorf("unexpected claim %q", st.Claim)
	}
	if st.Domain == "" {
		return st, errors.New("statement names no domain")
	}
	if st.KeyURL != KeyURLFor(st.Domain) {
		return st, fmt.Errorf("statement's key_url %q does not belong to %s", st.KeyURL, st.Domain)
	}

	issued, err := time.Parse(time.RFC3339, st.IssuedAt)
	if err != nil {
		return st, fmt.Errorf("unreadable issued_at %q", st.IssuedAt)
	}
	expires, err := time.Parse(time.RFC3339, st.ExpiresAt)
	if err != nil {
		return st, fmt.Errorf("unreadable expires_at %q", st.ExpiresAt)
	}
	if !expires.After(issued) {
		return st, errors.New("statement expires before it was issued")
	}
	if now.After(expires) {
		return st, ErrExpired
	}
	if issued.After(now.Add(clockSkew)) {
		return st, errors.New("statement is dated in the future")
	}
	return st, nil
}

// b64 is the encoding both segments use: base64url, unpadded, as in JWS.
var b64 = base64.RawURLEncoding

func split(blob string) (segment string, sig []byte, err error) {
	blob = strings.TrimSpace(blob)
	parts := strings.Split(blob, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, errors.New("not an attestation: expected payload.signature")
	}
	sig, err = b64.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("signature is not base64url: %w", err)
	}
	return parts[0], sig, nil
}

func decodePayload(segment string) (Statement, error) {
	raw, err := b64.DecodeString(segment)
	if err != nil {
		return Statement{}, fmt.Errorf("payload is not base64url: %w", err)
	}
	var st Statement
	if err := json.Unmarshal(raw, &st); err != nil {
		return Statement{}, fmt.Errorf("payload is not JSON: %w", err)
	}
	return st, nil
}

// parsePrivateKey accepts the PKCS#1 PEM that ap.GenerateKeyPair writes,
// and PKCS#8 as well, since an operator supplying a key by hand may have
// either.
func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return key, nil
}

func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("public key is not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not RSA")
	}
	return key, nil
}
