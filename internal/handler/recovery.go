package handler

import (
	"encoding/json"
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
			http.Error(w, `{"error":"too many attempts — wait a couple of minutes"}`, http.StatusTooManyRequests)
			return
		}

		user, err := auth.RedeemRecoveryCode(db, req.Username, req.Code)
		if err != nil {
			// One message for every failure shape; see errInvalidRecovery.
			http.Error(w, `{"error":"invalid username or recovery code"}`, http.StatusBadRequest)
			return
		}

		sessionToken, err := auth.CreateSession(db, user.ID, ip)
		if err != nil {
			http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
			return
		}

		auth.SetSessionCookie(w, sessionToken)
		auth.LogAuditEvent(db, user.ID, "user.login", "user", user.ID, `{"method":"recovery_code"}`, ip)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
