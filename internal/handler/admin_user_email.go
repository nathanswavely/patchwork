package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// SetUserEmail handles PUT /api/v1/admin/users/{id}/email (docs/adr/072).
//
// An account with no passkey, no recovery codes and no email address cannot
// be signed into at all, and until this existed the only repair was an
// UPDATE typed against the production database over SSH. Signup now hands
// every account a way back in, but that does nothing for the accounts
// already in that state, or for an address someone typed wrong.
//
// It is deliberately not a field on `PATCH /api/v1/admin/users/{id}`. Setting
// an address is *not* the same kind of act as suspending or granting a role:
// it points an account at a mailbox, and whoever holds that mailbox can
// magic-link into the account. That makes it the same shape as promotion to
// instance admin, so it takes the same step-up gate (docs/adr/017) — mounted
// on the whole route rather than checked per field, since there is only one
// field and it is the dangerous one.
func SetUserEmail(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		targetID := r.PathValue("id")
		if targetID == "" {
			http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Normalize before comparing or storing, through the same function
		// every other entry point uses (docs/adr/071): the stored form has to
		// be the form the person will type at the sign-in page, because that
		// lookup is an exact match.
		email, err := auth.NormalizeEmail(req.Email)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%s}`, jsonString(err.Error())), http.StatusBadRequest)
			return
		}

		var username, displayName string
		var oldEmail sql.NullString
		switch err = db.QueryRow(
			`SELECT username, display_name, email FROM users WHERE id = ?`, targetID,
		).Scan(&username, &displayName, &oldEmail); {
		case err == sql.ErrNoRows:
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		case err != nil:
			http.Error(w, `{"error":"failed to load user"}`, http.StatusInternalServerError)
			return
		}

		if oldEmail.Valid && oldEmail.String == email {
			// Already there. Not worth an audit entry claiming a change, nor
			// two emails announcing one.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "email": email})
			return
		}

		// users.email is UNIQUE, so the write would fail anyway — but it
		// would fail as a constraint violation, and "UNIQUE constraint
		// failed" is not an answer. Check case-insensitively as well: rows
		// written before normalization existed can hold a mixed-case address
		// that the UNIQUE index would happily let a lowercase twin sit
		// beside, which is exactly the collision this is meant to prevent.
		var holder string
		switch err := db.QueryRow(
			`SELECT username FROM users WHERE lower(email) = ? AND id != ?`, email, targetID,
		).Scan(&holder); {
		case err == nil:
			http.Error(w, fmt.Sprintf(
				`{"error":%s}`,
				jsonString(fmt.Sprintf("%s already uses that address — an address belongs to one account", holder)),
			), http.StatusConflict)
			return
		case err != sql.ErrNoRows:
			http.Error(w, `{"error":"failed to check the address"}`, http.StatusInternalServerError)
			return
		}

		result, err := db.Exec(
			`UPDATE users SET email = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			email, targetID,
		)
		if err != nil {
			// Lost a race with a concurrent write to the same address.
			if strings.Contains(err.Error(), "UNIQUE") {
				http.Error(w, `{"error":"another account already uses that address"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"error":"failed to set the email address"}`, http.StatusInternalServerError)
			return
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		// Its own action name, not admin.user_update: this is the entry
		// someone goes looking for when asking whether an admin pointed an
		// account at a mailbox they control, and it has to name both
		// addresses to answer that.
		meta, _ := json.Marshal(map[string]string{
			"username":  username,
			"old_email": oldEmail.String,
			"new_email": email,
		})
		auth.LogAuditEvent(db, adminUser.ID, "admin.user_email_set", "user", targetID, string(meta), clientIP(r))

		CreateNotification(db, targetID, "account.email_changed", "Email Address Changed",
			fmt.Sprintf("An instance admin set the email address on your account to %s. If you did not ask for this, contact the admins.", email),
			"/settings")

		notifyEmailChanged(db, cfg, displayName, oldEmail.String, email)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "email": email})
	}
}

// notifyEmailChanged writes to the address that is losing the account as well
// as the one gaining it. The new address alone would tell only whoever now
// holds the mailbox; the old one is the person who can say this was wrong.
//
// It goes out directly rather than through the notification channels, which
// resolve the recipient by user ID and would therefore only ever reach the
// new address — and which are preference-gated, where a notice that your
// account changed hands is not something to have opted out of.
func notifyEmailChanged(db *database.DB, cfg *config.Config, displayName, oldEmail, newEmail string) {
	if cfg == nil || !cfg.SMTP.Configured() {
		return
	}

	recipients := []string{newEmail}
	if oldEmail != "" && oldEmail != newEmail {
		recipients = append(recipients, oldEmail)
	}

	instance := settings.EffectiveName(db, cfg)
	settingsURL := weblink.Absolute(cfg.Instance.Domain, "/settings")

	was := "had no email address on file"
	if oldEmail != "" {
		was = fmt.Sprintf("was %s", oldEmail)
	}
	body := fmt.Sprintf(
		"Hello %s,\n\nAn admin on %s changed the email address on your account.\n\nIt %s. It is now %s.\n\nSigning in with a magic link will now send that link to the new address.\n\nIf you did not ask for this, reply to this message or contact the admins of %s — this note went to both the old and the new address on purpose.\n\n%s\n",
		displayName, instance, was, newEmail, instance, settingsURL,
	)

	smtp := cfg.SMTP
	for _, to := range recipients {
		msg := fmt.Sprintf(
			"From: %s\r\nTo: %s\r\nSubject: The email address on your %s account changed\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
			smtp.From, to, instance, body,
		)
		// Off the request: SMTP is somebody else's server and an admin
		// waiting on it would learn nothing useful from the delay.
		go func(to string, msg []byte) {
			if err := mail.Send(smtp, []string{to}, msg); err != nil {
				log.Printf("admin: email-change notice to %s failed: %v", to, err)
			}
		}(to, []byte(msg))
	}
}
