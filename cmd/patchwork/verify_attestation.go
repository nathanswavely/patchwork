package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/attest"
)

// runVerifyAttestation checks an admin attestation blob (docs/adr/087) and
// exits 0 if it stands, 1 if it does not.
//
// It needs no config and no database — it is meant for the other side of the
// exchange, a hosting provider or a directory holding a blob and a domain.
// The one thing it will not do is take the verifier's word for which key to
// use: the key comes from the domain the statement claims, fetched over
// HTTPS, or from a file the operator names. Fetching from a URL the blob
// chose would let anyone with an RSA key prove they run any domain.
func runVerifyAttestation(blob, keyFile string) {
	blob = strings.TrimSpace(blob)

	// Parse first so a person looking at a rejected blob can still read what
	// it claimed. This is display only; nothing below trusts it.
	claimed, err := attest.Parse(blob)
	if err != nil {
		fmt.Fprintf(os.Stderr, "not an attestation: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("claim:      %s\n", claimed.Claim)
	fmt.Printf("domain:     %s\n", claimed.Domain)
	fmt.Printf("nonce:      %s\n", claimed.Nonce)
	fmt.Printf("issued at:  %s\n", claimed.IssuedAt)
	fmt.Printf("expires at: %s\n", claimed.ExpiresAt)

	var publicKey string
	if keyFile != "" {
		raw, err := os.ReadFile(keyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read key file: %v\n", err)
			os.Exit(1)
		}
		publicKey = string(raw)
		fmt.Printf("key:        %s\n", keyFile)
	} else {
		// Built from the claimed domain, not read out of the blob — same
		// URL the statement's key_url must equal, but derived here so a
		// forged key_url cannot steer the fetch even if Verify were skipped.
		url := attest.KeyURLFor(claimed.Domain)
		fmt.Printf("key:        %s\n", url)
		publicKey, err = fetchAttestationKey(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nFAIL: %v\n", err)
			os.Exit(1)
		}
	}

	statement, err := attest.Verify(blob, publicKey, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nFAIL: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nOK: an admin of %s signed the nonce %q at %s.\n",
		statement.Domain, statement.Nonce, statement.IssuedAt)
	fmt.Println("This says the role, not the person. It names nobody, and it proves nothing about who held the keyboard.")
	os.Exit(0)
}

func fetchAttestationKey(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: %s", url, resp.Status)
	}

	var body struct {
		Algorithm string `json:"algorithm"`
		PublicKey string `json:"public_key"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("%s did not answer with a key document: %w", url, err)
	}
	if body.PublicKey == "" {
		return "", fmt.Errorf("%s served no public_key", url)
	}
	// The algorithm is checked, never obeyed: verification is hardcoded to
	// RS256 either way. This only catches an instance that has moved on to
	// something else, so the failure reads as "newer format" rather than
	// "bad signature".
	if body.Algorithm != "" && body.Algorithm != attest.Algorithm {
		return "", fmt.Errorf("%s signs with %s, this build verifies %s", url, body.Algorithm, attest.Algorithm)
	}
	return body.PublicKey, nil
}
