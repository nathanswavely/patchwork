// Package atproto resolves an AT Protocol identity for a domain a patch has
// already proved it controls (docs/adr/062).
//
// It is a client and nothing else, per ADR 058's first constraint: two
// lookups against a domain, no relay, no AppView, no repository. Only
// `did:web` is accepted — a `did:plc` resolves through plc.directory, a
// registry the community does not own, which is the hostage relationship
// ADR 060 objected to moved one level out.
package atproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrNotDIDWeb is returned when a handle resolves to a DID method this
// project deliberately does not accept. Callers surface the reason rather
// than a generic failure: being told "did:plc is not accepted here" is
// actionable, and being told "verification failed" is not.
var ErrNotDIDWeb = errors.New("handle resolves to a DID that is not did:web")

const (
	handleTXTPrefix = "did="
	maxDocBytes     = 64 * 1024
)

// Doc is the subset of a DID document this project reads.
type Doc struct {
	ID          string    `json:"id"`
	AlsoKnownAs []string  `json:"alsoKnownAs"`
	Service     []Service `json:"service"`
}

// Service is a DID document service entry. The one this project wants is
// the account's personal data server, which is where its records live.
type Service struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// PDSEndpoint returns the account's personal data server, the host that
// answers XRPC reads for this repository (docs/adr/064).
func (d *Doc) PDSEndpoint() (string, error) {
	if d == nil {
		return "", errors.New("no DID document")
	}
	for _, svc := range d.Service {
		if svc.Type == "AtprotoPersonalDataServer" && svc.ServiceEndpoint != "" {
			return strings.TrimSuffix(svc.ServiceEndpoint, "/"), nil
		}
	}
	return "", errors.New("DID document names no personal data server")
}

// LookupTXT and Get are the two external seams, swappable in tests. Get is
// expected to be SSRF-guarded by the caller — every URL it receives is built
// from a domain someone outside the instance chose.
type Resolver struct {
	LookupTXT func(domain string) ([]string, error)
	Get       func(url string) (*http.Response, error)
}

// ResolveHandle returns the DID a handle points at, trying the TXT record
// first and the well-known file second. A domain needs only one of them.
//
// The handle is the domain: atproto handles are domains, and the patch has
// a vetted one already (docs/adr/062 decision 1).
func (r Resolver) ResolveHandle(domain string) (string, error) {
	if did, err := r.handleFromTXT(domain); err == nil && did != "" {
		return did, nil
	}
	return r.handleFromWellKnown(domain)
}

func (r Resolver) handleFromTXT(domain string) (string, error) {
	records, err := r.LookupTXT("_atproto." + domain)
	if err != nil {
		return "", err
	}
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if strings.HasPrefix(rec, handleTXTPrefix) {
			return strings.TrimSpace(strings.TrimPrefix(rec, handleTXTPrefix)), nil
		}
	}
	return "", errors.New("no _atproto TXT record")
}

func (r Resolver) handleFromWellKnown(domain string) (string, error) {
	body, err := r.fetch("https://" + domain + "/.well-known/atproto-did")
	if err != nil {
		return "", err
	}
	did := strings.TrimSpace(string(body))
	if !strings.HasPrefix(did, "did:") {
		return "", errors.New("well-known file does not contain a DID")
	}
	return did, nil
}

// DocURLFor maps a did:web identifier to the URL its document is served
// from. Bare `did:web:example.com` is the root well-known path; extra
// colon-separated segments are path segments, per the did:web method.
func DocURLFor(did string) (string, error) {
	if !strings.HasPrefix(did, "did:web:") {
		return "", ErrNotDIDWeb
	}
	rest := strings.TrimPrefix(did, "did:web:")
	if rest == "" {
		return "", errors.New("did:web with no domain")
	}
	parts := strings.Split(rest, ":")
	// A did:web domain may carry a percent-encoded port; nothing else in a
	// segment should be encoded, and decoding the rest would let a crafted
	// DID walk the path.
	for _, p := range parts {
		if p == "" || strings.Contains(p, "/") {
			return "", errors.New("malformed did:web")
		}
	}
	host := strings.ReplaceAll(parts[0], "%3A", ":")
	if len(parts) == 1 {
		return "https://" + host + "/.well-known/did.json", nil
	}
	return "https://" + host + "/" + strings.Join(parts[1:], "/") + "/did.json", nil
}

// plcDirectory resolves did:plc identifiers. Unlike did:web, which is
// served from the domain itself, a did:plc means nothing without asking
// this registry — which is exactly why docs/adr/062 refuses did:plc for a
// patch's OWN identity. Reading somebody's calendar adopts nothing, so it
// is allowed here (docs/adr/064 decision 2).
const plcDirectory = "https://plc.directory"

// ResolveDoc fetches and parses a DID document, for did:web or did:plc.
func (r Resolver) ResolveDoc(did string) (*Doc, error) {
	var url string
	var err error
	switch {
	case strings.HasPrefix(did, "did:plc:"):
		url = plcDirectory + "/" + did
	default:
		url, err = DocURLFor(did)
	}
	if err != nil {
		return nil, err
	}
	body, err := r.fetch(url)
	if err != nil {
		return nil, err
	}
	var doc Doc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("DID document is not valid JSON: %w", err)
	}
	return &doc, nil
}

// VerifyBinding checks the half of the proof the DNS side cannot give.
//
// Either direction alone is forgeable: anyone who can publish a DNS record
// can point a handle at somebody else's DID, and any DID document can name
// a handle it does not hold. Both together are the binding.
func VerifyBinding(doc *Doc, did, domain string) error {
	if doc == nil {
		return errors.New("no DID document")
	}
	if !strings.EqualFold(doc.ID, did) {
		return fmt.Errorf("DID document is for %s, not %s", doc.ID, did)
	}
	want := "at://" + domain
	for _, aka := range doc.AlsoKnownAs {
		if strings.EqualFold(strings.TrimSpace(aka), want) {
			return nil
		}
	}
	return fmt.Errorf("DID document does not claim %s in alsoKnownAs", want)
}

// Verify runs the whole bidirectional check for a domain and returns the
// DID it is bound to.
func (r Resolver) Verify(domain string) (string, error) {
	did, err := r.ResolveHandle(domain)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(did, "did:web:") {
		return "", fmt.Errorf("%w: %s", ErrNotDIDWeb, did)
	}
	doc, err := r.ResolveDoc(did)
	if err != nil {
		return "", err
	}
	if err := VerifyBinding(doc, did, domain); err != nil {
		return "", err
	}
	return did, nil
}

func (r Resolver) fetch(url string) ([]byte, error) {
	resp, err := r.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDocBytes))
}
