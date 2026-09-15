package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// GenerateRecoveryCodes handles POST /api/v1/auth/recovery-codes.
// Returns a fresh batch of raw codes — the only response that ever contains
// them — and replaces any earlier batch (docs/adr/020).
func GenerateRecoveryCodes(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		codes, err := auth.GenerateRecoveryCodes(db, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to generate recovery codes"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "recovery_codes.generate", "user", user.ID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"codes": codes,
		})
	}
}

// RecoveryCodeStatus handles GET /api/v1/auth/recovery-codes.
// Reports counts only — the codes themselves are shown once, at generation.
func RecoveryCodeStatus(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		total, remaining, err := auth.CountRecoveryCodes(db, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to load recovery code status"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{
			"total":     total,
			"remaining": remaining,
		})
	}
}

// RedeemRecoveryCode handles POST /api/v1/auth/recovery with
// {username, code}. A valid pair burns the code and signs the person in —
// the lost-passkey path that needs no email (docs/adr/020).
func RedeemRecoveryCode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Code     string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Code == "" {
			http.Error(w, `{"error":"username and code are required"}`, http.StatusBadRequest)
			return
		}

		ip := clientIP(r)

		if err := middleware.CheckRecoveryRedeemRate(req.Username, ip); err != nil {
			w.Header().Set("Retry-After", "120")
			http.Error(w, `{"error":"too many attempts. Wait a couple of minutes"}`, http.StatusTooManyRequests)
			return
		}

		user, err := auth.RedeemRecoveryCode(db, req.Username, req.Code)
		if err != nil {
			// One message for every failure shape; see errInvalidRecovery.
			http.Error(w, `{"error":"invalid username or recovery code"}`, http.StatusBadRequest)
			return
		}

		sessionToken, err := auth.CreateSession(db, user.ID, ip, r.UserAgent())
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, sessionToken)
		// Redeeming a code is itself the proof step-up asks for, so the
		// window opens on arrival (docs/adr/099). Without this, somebody
		// with no passkey who signed in *because* they had no passkey would
		// land needing a second code they cannot yet use — the batch they
		// just made postdates this session by design.
		if _, err := auth.GrantSudo(db, sessionToken); err != nil {
			log.Printf("recovery sign-in: could not open the step-up window: %v", err)
		}
		auth.LogAuditEvent(db, user.ID, "user.login", "user", user.ID, `{"method":"recovery_code","step_up":true}`, ip)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
