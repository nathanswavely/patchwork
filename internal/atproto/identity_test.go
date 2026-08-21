package atproto_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/atproto"
)

func resp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

// A resolver whose two seams are canned maps.
func stub(txt map[string][]string, pages map[string]string) atproto.Resolver {
	return atproto.Resolver{
		LookupTXT: func(d string) ([]string, error) {
			if r, ok := txt[d]; ok {
				return r, nil
			}
			return nil, errors.New("NXDOMAIN")
		},
		Get: func(url string) (*http.Response, error) {
			if b, ok := pages[url]; ok {
				return resp(200, b), nil
			}
			return resp(404, ""), nil
		},
	}
}

const docFor = `{"id":"did:web:tellus.example","alsoKnownAs":["at://tellus.example"]}`

func TestVerify_TXTRoute(t *testing.T) {
	r := stub(
		map[string][]string{"_atproto.tellus.example": {"did=did:web:tellus.example"}},
		map[string]string{"https://tellus.example/.well-known/did.json": docFor},
	)
	did, err := r.Verify("tellus.example")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if did != "did:web:tellus.example" {
		t.Errorf("got %q", did)
	}
}

// A domain needs only one of the two routes; the well-known file is the
// fallback for anyone who can serve a file but not edit DNS.
func TestVerify_WellKnownRoute(t *testing.T) {
	r := stub(nil, map[string]string{
		"https://tellus.example/.well-known/atproto-did": "did:web:tellus.example\n",
		"https://tellus.example/.well-known/did.json":    docFor,
	})
	if _, err := r.Verify("tellus.example"); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// docs/adr/062 decision 2. The common case in the wider world, refused on
// purpose, and the caller must be able to tell this apart from a typo.
func TestVerify_RefusesDIDPLC(t *testing.T) {
	r := stub(
		map[string][]string{"_atproto.tellus.example": {"did=did:plc:abc123"}},
		nil,
	)
	_, err := r.Verify("tellus.example")
	if !errors.Is(err, atproto.ErrNotDIDWeb) {
		t.Fatalf("want ErrNotDIDWeb, got %v", err)
	}
}

// The forgery the second direction exists to stop: anyone who can publish a
// DNS record can point their handle at somebody else's DID.
func TestVerify_RejectsHandlePointingAtAStrangersDID(t *testing.T) {
	r := stub(
		map[string][]string{"_atproto.impostor.example": {"did=did:web:tellus.example"}},
		map[string]string{"https://tellus.example/.well-known/did.json": docFor},
	)
	_, err := r.Verify("impostor.example")
	if err == nil {
		t.Fatal("a handle claiming another domain's DID must not verify")
	}
	if !strings.Contains(err.Error(), "alsoKnownAs") {
		t.Errorf("want the binding named in the error, got: %v", err)
	}
}

// The mirror forgery: a DID document naming a handle that does not point back.
func TestVerify_RejectsDocumentClaimingAHandleItDoesNotHold(t *testing.T) {
	doc := `{"id":"did:web:tellus.example","alsoKnownAs":["at://someone-else.example"]}`
	r := stub(
		map[string][]string{"_atproto.tellus.example": {"did=did:web:tellus.example"}},
		map[string]string{"https://tellus.example/.well-known/did.json": doc},
	)
	if _, err := r.Verify("tellus.example"); err == nil {
		t.Fatal("a document that does not claim the handle must not verify")
	}
}

// A document served at one DID's URL but self-identifying as another.
func TestVerify_RejectsDocumentForADifferentDID(t *testing.T) {
	doc := `{"id":"did:web:elsewhere.example","alsoKnownAs":["at://tellus.example"]}`
	r := stub(
		map[string][]string{"_atproto.tellus.example": {"did=did:web:tellus.example"}},
		map[string]string{"https://tellus.example/.well-known/did.json": doc},
	)
	if _, err := r.Verify("tellus.example"); err == nil {
		t.Fatal("a document identifying as a different DID must not verify")
	}
}

func TestDocURLFor(t *testing.T) {
	cases := map[string]string{
		"did:web:tellus.example":           "https://tellus.example/.well-known/did.json",
		"did:web:tellus.example:patches:1": "https://tellus.example/patches/1/did.json",
		"did:web:tellus.example%3A8443":    "https://tellus.example:8443/.well-known/did.json",
	}
	for did, want := range cases {
		got, err := atproto.DocURLFor(did)
		if err != nil {
			t.Errorf("%s: %v", did, err)
			continue
		}
		if got != want {
			t.Errorf("%s → %s, want %s", did, got, want)
		}
	}

	for _, bad := range []string{"did:plc:abc", "did:web:", "not-a-did", "did:web:a::b"} {
		if _, err := atproto.DocURLFor(bad); err == nil {
			t.Errorf("%q should not produce a URL", bad)
		}
	}
}
