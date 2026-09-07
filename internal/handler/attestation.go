package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/attest"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// Proving the admin role to an outside party (docs/adr/087).
//
// A hosting provider re-pointing a billing contact, or a directory checking a
// submission, needs to know that whoever is writing to them administers the
// quilt they say they do. Two endpoints answer that between them: an admin
// signs the outsider's nonce here, and the outsider fetches the public half
// from the quilt's own domain.
//
// What the pair discloses is bounded on purpose. The statement names the
// domain and the claim and nothing else — no username, no email, no user id,
// no count — because ADR 023 refused to publish the admin roster and a proof
// of the role must not become the roster by another route. It says an admin
// of this quilt signed this string at this time. It is not an identity.

// AttestationKey serves GET /api/v1/instance/attestation-key.
//
// Public, unauthenticated and cacheable: it is the half a verifier needs, and
// a verifier is by definition somebody with no account here. It is mounted
// outside the federation gate, unlike /ap/instance, because whether a quilt
// federates has nothing to do with whether its admin can prove the role — and
// a key reachable only when federation happens to be on is a proof that stops
// working when an instance turns federation off.
func AttestationKey(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, publicKey, err := ap.InstanceActorKeys(db)
		if err != nil || publicKey == "" {
			http.Error(w, `{"error":"this quilt has no instance key yet"}`, http.StatusServiceUnavailable)
			return
		}

		// An hour. The key changes only when an instance is rebuilt from
		// nothing, and a verifier re-fetching it per attestation is wasted
		// traffic on somebody's Pi.
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"algorithm":  attest.Algorithm,
			"domain":     cfg.Instance.Domain,
			"public_key": publicKey,
			"format":     "base64url(payload).base64url(signature), signed over the first segment's bytes",
		})
	}
}

// IssueAttestation handles POST /api/v1/admin/attestation.
//
// Mounted behind AdminRequired and SudoRequired. Step-up (docs/adr/017)
// because this is the shape of the gates that hand something outward —
// promotion, export, setting an address: holding the cookie proves identity,
// and what an outward-facing act needs is proof of presence. A blob signed
// from a stolen laptop is a credential somebody else can spend.
func IssueAttestation(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		if admin == nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		// Signing is cheap but not free, and an admin has no honest reason to
		// ask for more than a handful. Keyed per admin, so one busy account
		// cannot spend everyone's allowance.
		if err := middleware.CheckAttestationRate(admin.ID); err != nil {
			http.Error(w, `{"error":"too many attestations — wait a minute and try again"}`, http.StatusTooManyRequests)
			return
		}

		var req struct {
			Nonce string `json:"nonce"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if err := attest.ValidateNonce(req.Nonce); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%s}`, jsonString(err.Error())), http.StatusBadRequest)
			return
		}

		// A quilt that has never federated may have no instance actor yet:
		// the startup pass mints one, but an instance upgrading into this
		// feature reaches its first sign-in before its next restart. Minting
		// here costs one keypair, once, behind the admin and step-up gates.
		if err := ap.EnsureInstanceActor(db, ap.GetDomain()); err != nil {
			http.Error(w, `{"error":"failed to load the instance key"}`, http.StatusInternalServerError)
			return
		}
		privateKey, err := ap.InstanceActorPrivateKey(db)
		if err != nil {
			http.Error(w, `{"error":"failed to load the instance key"}`, http.StatusInternalServerError)
			return
		}

		statement := attest.NewStatement(cfg.Instance.Domain, req.Nonce, time.Now())
		blob, err := attest.Sign(privateKey, statement)
		if err != nil {
			http.Error(w, `{"error":"failed to sign the attestation"}`, http.StatusInternalServerError)
			return
		}

		// The nonce is in the audit row on purpose: a later dispute is
		// always "did anyone here sign this string", and the log is the only
		// place that can answer it. The blob itself is not stored — it is
		// reproducible from the key and worthless after fifteen minutes, and
		// a table of live proofs is a thing to steal.
		meta, _ := json.Marshal(map[string]string{
			"nonce":      statement.Nonce,
			"expires_at": statement.ExpiresAt,
		})
		auth.LogAuditEvent(db, admin.ID, "admin.attestation_issued", "instance", cfg.Instance.Domain, string(meta), clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"attestation": blob,
			"statement":   statement,
			"algorithm":   attest.Algorithm,
			"key_url":     statement.KeyURL,
		})
	}
}
