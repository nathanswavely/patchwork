package ap

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
	"strings"
	"time"
)

// GenerateKeyPair creates a 2048-bit RSA keypair and returns PEM-encoded strings.
func GenerateKeyPair() (publicKeyPEM, privateKeyPEM string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	// Encode private key
	privBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})

	// Encode public key
	pubBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	return string(pubPEM), string(privPEM), nil
}

// SignRequest signs an HTTP request using the actor's private key.
// Implements HTTP Signatures (draft-cavage-http-signatures-12).
func SignRequest(req *http.Request, keyID string, privateKeyPEM string) error {
	// Parse private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to parse private key PEM")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Set Date header if not present
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}

	// Build the signing string
	// Standard headers to sign: (request-target) host date
	headers := []string{"(request-target)", "host", "date"}
	if req.Header.Get("Digest") != "" {
		headers = append(headers, "digest")
	}

	var signParts []string
	for _, h := range headers {
		switch h {
		case "(request-target)":
			signParts = append(signParts, fmt.Sprintf("(request-target): %s %s", strings.ToLower(req.Method), req.URL.RequestURI()))
		default:
			signParts = append(signParts, fmt.Sprintf("%s: %s", h, signatureHeaderValue(req, h)))
		}
	}
	signingString := strings.Join(signParts, "\n")

	// Sign with RSA-SHA256
	hash := sha256.Sum256([]byte(signingString))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return fmt.Errorf("failed to sign: %w", err)
	}

	// Build Signature header
	sig := fmt.Sprintf(`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		keyID,
		strings.Join(headers, " "),
		base64.StdEncoding.EncodeToString(signature),
	)
	req.Header.Set("Signature", sig)

	return nil
}

// VerifySignature verifies the HTTP Signature on an incoming request.
// publicKeyPEM is the PEM-encoded public key of the remote actor.
func VerifySignature(req *http.Request, publicKeyPEM string) error {
	// Parse Signature header
	sigHeader := req.Header.Get("Signature")
	if sigHeader == "" {
		return fmt.Errorf("missing Signature header")
	}

	params := parseSignatureHeader(sigHeader)

	headersStr := params["headers"]
	if headersStr == "" {
		headersStr = "date"
	}
	headers := strings.Split(headersStr, " ")

	// A signature only proves what it signs, so the sender does not get to
	// choose how little that is.
	covered := make(map[string]bool, len(headers))
	for _, h := range headers {
		covered[strings.ToLower(strings.TrimSpace(h))] = true
	}
	for _, want := range requiredSignedHeaders(req.Method) {
		if !covered[want] {
			return fmt.Errorf("signature does not cover %s", want)
		}
	}

	// Rebuild signing string
	var signParts []string
	for _, h := range headers {
		switch h {
		case "(request-target)":
			signParts = append(signParts, fmt.Sprintf("(request-target): %s %s", strings.ToLower(req.Method), req.URL.RequestURI()))
		default:
			signParts = append(signParts, fmt.Sprintf("%s: %s", h, signatureHeaderValue(req, h)))
		}
	}
	signingString := strings.Join(signParts, "\n")

	// Decode signature
	sigBytes, err := base64.StdEncoding.DecodeString(params["signature"])
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Parse public key
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to parse public key PEM")
	}
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}
	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}

	// Verify
	hash := sha256.Sum256([]byte(signingString))
	return rsa.VerifyPKCS1v15(rsaPubKey, crypto.SHA256, hash[:], sigBytes)
}

// requiredSignedHeaders names the headers an inbound signature has to cover
// before the rest of the verification means anything.
//
// The verifier used to rebuild the signing string from whatever list the
// sender supplied, defaulting to "date" alone when the list was absent. Every
// check downstream then rested on a value nobody had signed: checkDateSkew
// reads the Date header and treats it as authentic, which is only true if the
// signature covers it; the inbox compares Digest against the body only when a
// Digest header happens to be present, so a body is unauthenticated without
// it; and without (request-target), a POST captured for one inbox can be
// replayed into another. The compensating checks did hold this closed in
// practice. Naming the set is what makes them true by construction rather
// than by the good manners of the sender.
//
// The set is what our own SignRequest covers and what mainstream fediverse
// implementations send: (request-target), host and date always, plus digest
// on any method that carries a body.
func requiredSignedHeaders(method string) []string {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return []string{"(request-target)", "host", "date", "digest"}
	default:
		return []string{"(request-target)", "host", "date"}
	}
}

// signatureHeaderValue returns the value to use for a given header name when
// building the HTTP Signatures signing string. The "host" pseudo-header needs
// special handling: Go does not expose it via Header.Get — on server-side
// requests the host is promoted to req.Host, and on client-side requests it
// lives in req.URL.Host. Without this, the signed/verified host is empty and
// interop with Mastodon and other fediverse servers fails.
func signatureHeaderValue(req *http.Request, header string) string {
	if strings.EqualFold(header, "host") {
		if req.Host != "" {
			return req.Host
		}
		return req.URL.Host
	}
	return req.Header.Get(header)
}

// parseSignatureHeader parses the Signature header into key-value pairs.
func parseSignatureHeader(header string) map[string]string {
	params := make(map[string]string)
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.Trim(strings.TrimSpace(part[eq+1:]), `"`)
		params[key] = val
	}
	return params
}
