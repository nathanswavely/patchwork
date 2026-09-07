package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Self-serve account deletion (docs/adr/086).
//
// "Delete my account" cannot mean `DELETE FROM users` here. Most foreign keys
// into users(id) are RESTRICT, and they are RESTRICT for a reason: votes,
// proposals, attestations and notices are a community's record of what it
// decided, and a tally that quietly loses a voter is not a record anybody can
// audit. So deletion erases the person and keeps the acts — the row survives
// as a tombstone with every identity column emptied, `deleted_at` set, and
// the username retired rather than freed.

// soleAdminPatch is a patch the person cannot walk out of: they are its only
// active admin, and no succession would fire to replace them.
type soleAdminPatch struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// soleAdminBlockers lists the patches that stand between this person and
// deleting their account.
//
// It is the same floor `LeaveNode` enforces, applied to every patch at once:
// deleting an account is leaving all of them. A maintainer patch with a
// designated successor is not a blocker, because on that patch leaving is
// precisely what hands it over (docs/adr/051) — refusing there would make the
// account-deletion rule stricter than the leave rule for no reason anybody
// could explain.
//
// A suspended or pending membership is not an admin present: the point of the
// count is "is there somebody who can run this patch tomorrow", and a
// suspended account cannot sign in.
func soleAdminBlockers(db *database.DB, userID string) ([]soleAdminPatch, error) {
	rows, err := db.Query(`
		SELECT n.id, n.slug, n.name
		FROM memberships m
		JOIN nodes n ON n.id = m.node_id
		WHERE m.user_id = ? AND m.role = 'admin' AND m.status = 'active'
		  AND n.removed_at IS NULL AND n.status != 'archived'
		  AND (
		    SELECT COUNT(*) FROM memberships om
		    JOIN users ou ON ou.id = om.user_id
		    WHERE om.node_id = n.id AND om.role = 'admin' AND om.status = 'active'
		      AND ou.suspended_at IS NULL AND ou.deleted_at IS NULL
		  ) <= 1
		ORDER BY n.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct{ id, slug, name string }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.slug, &c.name); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	blockers := []soleAdminPatch{}
	for _, c := range candidates {
		if leadershipModel(db, c.id) == "maintainer" {
			if s := designatedSuccessor(db, c.id); s != "" && s != userID {
				continue
			}
		}
		blockers = append(blockers, soleAdminPatch{Slug: c.slug, Name: c.name})
	}
	return blockers, nil
}

// otherInstanceAdmins counts instance admins other than this one who could
// still administer the instance tomorrow.
func otherInstanceAdmins(db *database.DB, userID string) int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users
	             WHERE role = 'admin' AND id != ? AND suspended_at IS NULL AND deleted_at IS NULL`,
		userID).Scan(&n)
	return n
}

// DeleteMyAccount handles DELETE /api/v1/users/me (docs/adr/086).
//
// Step-up gated on the route, the way the other irreversible actions are
// (docs/adr/017), and confirmed by typing the username. There is no grace
// window: the step-up assertion is what stands between a stolen cookie and
// this button, and a cancel link would only be as good as the mailbox it
// arrived in.
func DeleteMyAccount(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			ConfirmUsername string `json:"confirm_username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ConfirmUsername) != user.Username {
			http.Error(w, `{"error":"type your username exactly to confirm"}`, http.StatusBadRequest)
			return
		}

		blockers, err := soleAdminBlockers(db, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to check your patches"}`, http.StatusInternalServerError)
			return
		}
		if len(blockers) > 0 {
			writeJSONStatus(w, http.StatusConflict, map[string]interface{}{
				"error":   "hand these patches to somebody else first — you are their only admin",
				"code":    "sole_admin",
				"patches": blockers,
			})
			return
		}

		if user.Role == "admin" && otherInstanceAdmins(db, user.ID) == 0 {
			writeJSONStatus(w, http.StatusConflict, map[string]interface{}{
				"error": "you are the only admin of this quilt — make somebody else an admin first",
				"code":  "last_instance_admin",
			})
			return
		}

		// Read what deletion is about to erase, while it is still there: the
		// address the confirmation goes to, the actor the fediverse knows,
		// and the inboxes that need telling.
		var (
			email       sql.NullString
			apID        sql.NullString
			displayName string
		)
		if err := db.QueryRow(
			`SELECT email, ap_id, display_name FROM users WHERE id = ?`, user.ID,
		).Scan(&email, &apID, &displayName); err != nil {
			http.Error(w, `{"error":"failed to load your account"}`, http.StatusInternalServerError)
			return
		}
		followerInboxes := actorFollowerInboxes(db, user.ID)
		remoteFollowed := remoteFollowTargets(db, user.ID)

		// The confirmation goes out before the address is erased. It is the
		// one piece of this that cannot be done afterwards.
		sendDeletionConfirmation(db, cfg, displayName, user.Username, email.String)

		// Succession first, and outside the erase: on a maintainer patch with
		// a named successor, leaving is the handover (docs/adr/051), and it
		// notifies the person taking over. soleAdminBlockers has already
		// established that every patch here either has another admin or has
		// somebody to succeed to.
		for _, s := range maintainerHandovers(db, user.ID) {
			succeedOnDeparture(db, r, s.id, s.slug, user.ID)
		}

		if err := eraseAccount(db, user.ID); err != nil {
			log.Printf("account deletion: erase %s: %v", user.ID, err)
			http.Error(w, `{"error":"failed to delete your account"}`, http.StatusInternalServerError)
			return
		}

		// Audited under the user namespace the person's own actions use
		// (user.logout, membership.leave), with the acting user recorded:
		// audit_log.user_id is ON DELETE SET NULL and the row is not deleted,
		// so the entry keeps pointing at the tombstone it describes.
		auth.LogAuditEvent(db, user.ID, "user.deleted", "user", user.ID,
			fmt.Sprintf(`{"username":%s}`, jsonString(user.Username)), clientIP(r))

		// Federation, after the erase so a delivery failure cannot leave a
		// half-deleted account behind. Both directions: tell the servers that
		// followed this person, and stop following on their behalf.
		if cfg != nil && cfg.Federation.Enabled {
			if apID.Valid && apID.String != "" {
				del := ap.BuildDeleteActor(apID.String)
				for _, inbox := range followerInboxes {
					if err := ap.QueueActivity(db, del, inbox); err != nil {
						log.Printf("account deletion: queue Delete to %s: %v", inbox, err)
					}
				}
			}
			for _, nodeAPID := range remoteFollowed {
				go relayUnfollow(db, nodeAPID)
			}
		}

		auth.ClearSessionCookie(w)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

// writeJSONStatus writes a JSON body with a status code. The refusals here
// carry a machine-readable code and a list of patches, which http.Error's
// hand-written JSON string cannot express.
func writeJSONStatus(w http.ResponseWriter, status int, body map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// actorFollowerInboxes returns the remote inboxes that follow this person's
// own actor, read before the rows are deleted.
func actorFollowerInboxes(db *database.DB, userID string) []string {
	rows, err := db.Query(
		`SELECT remote_inbox FROM ap_followers
		 WHERE local_actor_type = 'user' AND local_actor_id = ?
		   AND remote_inbox IS NOT NULL AND remote_inbox != ''`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	seen := map[string]bool{}
	var inboxes []string
	for rows.Next() {
		var inbox string
		if rows.Scan(&inbox) != nil || seen[inbox] {
			continue
		}
		seen[inbox] = true
		inboxes = append(inboxes, inbox)
	}
	return inboxes
}

// remoteFollowTargets returns the remote patch actors this person follows, so
// the instance actor's relayed Follow can be withdrawn once their rows are
// gone. relayUnfollow re-counts before sending, so a patch somebody else here
// also follows keeps its Follow (docs/adr/024).
func remoteFollowTargets(db *database.DB, userID string) []string {
	rows, err := db.Query(`SELECT node_ap_id FROM remote_follows WHERE user_id = ?`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var targets []string
	for rows.Next() {
		var apID string
		if rows.Scan(&apID) == nil && apID != "" {
			targets = append(targets, apID)
		}
	}
	return targets
}

// maintainerHandovers lists the maintainer patches where this person's
// departure should promote a named successor.
func maintainerHandovers(db *database.DB, userID string) []struct{ id, slug string } {
	rows, err := db.Query(`
		SELECT n.id, n.slug FROM memberships m JOIN nodes n ON n.id = m.node_id
		WHERE m.user_id = ? AND m.role = 'admin' AND m.status = 'active'
		  AND n.removed_at IS NULL AND n.designated_successor_id IS NOT NULL`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []struct{ id, slug string }
	for rows.Next() {
		var s struct{ id, slug string }
		if rows.Scan(&s.id, &s.slug) == nil {
			out = append(out, s)
		}
	}
	return out
}

// eraseAccount empties the person out of the tombstone and deletes every row
// that is theirs alone, in one transaction: a half-erased account is worse
// than either outcome.
//
// The three groups below are the whole decision. Emptied: identity. Deleted:
// rows that are the person and nothing else. Kept: the acts — votes, ballots,
// proposals, comments, notices, attestations, event and source authorship —
// which is why the row survives at all.
func eraseAccount(db *database.DB, userID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Rows that are purely the person's own. Nothing here is a community
	// record: they are credentials, delivery state, preferences, and the
	// relationships that end when the person does.
	//
	// election_candidates is on this list and votes are not, which looks
	// inconsistent until you read what each row is. A ballot is a decision
	// somebody made and it stays counted. A candidacy is a standing offer to
	// serve, and an account that no longer exists cannot serve — leaving it
	// would let a tombstone win a seat.
	//
	// A pending claim goes for the same reason: it asks an admin to hand a
	// patch to somebody who will not be there to receive it. Settled claims
	// stay as the record of a review, minus the address they carried.
	//
	// The contact card goes too, and it has to be said out loud here: a card
	// is the person, never an act, so nothing about keeping the record whole
	// argues for keeping a phone number. `contact_items.user_id` is declared
	// ON DELETE CASCADE, which never fires because this deletion keeps the
	// users row as a tombstone (docs/adr/086) — the same trap seats.holder_id
	// falls into below. Deleting the memberships above already ends every
	// disclosure, since both surfaces require an active member/admin row
	// (docs/adr/083), but ending the disclosure is not erasing the value.
	purges := []string{
		`DELETE FROM sessions WHERE user_id = ?`,
		`DELETE FROM credentials WHERE user_id = ?`,
		`DELETE FROM recovery_codes WHERE user_id = ?`,
		`DELETE FROM notifications WHERE user_id = ?`,
		`DELETE FROM notification_preferences WHERE user_id = ?`,
		`DELETE FROM memberships WHERE user_id = ?`,
		`DELETE FROM user_quilts WHERE user_id = ?`,
		`DELETE FROM remote_follows WHERE user_id = ?`,
		`DELETE FROM label_stewards WHERE user_id = ?`,
		`DELETE FROM election_candidates WHERE user_id = ?`,
		`DELETE FROM ap_followers WHERE local_actor_type = 'user' AND local_actor_id = ?`,
		`DELETE FROM claim_requests WHERE user_id = ? AND status = 'pending'`,
		// Shares first, though the FK would cascade them: this list is the
		// statement of what goes, and a reader should not have to know the
		// schema to see that it goes.
		`DELETE FROM contact_item_shares WHERE item_id IN (SELECT id FROM contact_items WHERE user_id = ?)`,
		`DELETE FROM contact_items WHERE user_id = ?`,
	}
	for _, q := range purges {
		if _, err := tx.Exec(q, userID); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}

	// Pointers at the person held by rows that are not theirs. seats.holder_id
	// and proposals.target_user_id are declared ON DELETE SET NULL, which
	// never fires because nothing is deleted, so the schema's intent is
	// carried out here by hand. A vacated seat is the holdover case the
	// election machinery already knows how to sit out (docs/adr/051).
	nulls := []string{
		`UPDATE seats SET holder_id = NULL WHERE holder_id = ?`,
		`UPDATE proposals SET target_user_id = NULL WHERE target_user_id = ?`,
		`UPDATE nodes SET designated_successor_id = NULL WHERE designated_successor_id = ?`,
		`UPDATE attestation_names SET user_id = NULL WHERE user_id = ?`,
		// A claim's email column is contact detail for a review, not part of
		// the record of what was decided.
		`UPDATE claim_requests SET email = '', verification_token = NULL WHERE user_id = ?`,
	}
	for _, q := range nulls {
		if _, err := tx.Exec(q, userID); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}

	// The tombstone. `username` is deliberately left in place: it is the
	// UNIQUE constraint that retires the handle, so nobody can register it
	// later and inherit the links pointing at somebody else's old profile.
	// Nothing renders it — the profile route 404s and every API surface
	// substitutes (deleted_accounts.go).
	//
	// `role` goes back to member: an account nobody holds is not an instance
	// admin, and leaving it would let a tombstone satisfy the last-admin
	// check for the next person who tries to leave.
	//
	// `moved_to` goes with the rest of the identity columns (docs/adr/090).
	// It is a sentence about where to find this person, and somebody who
	// deleted their account asked for the opposite of that.
	if _, err := tx.Exec(`
		UPDATE users SET
			email = NULL,
			display_name = '',
			bio = '',
			avatar_url = '',
			links = '[]',
			contact_phone = '', contact_email = '', contact_note = '',
			role = 'member',
			trusted_contributor = 0,
			hide_amended_linings = 0,
			start_on_my_quilt = 0,
			feed_secret_hash = NULL,
			moved_to = NULL,
			public_key = NULL,
			private_key = NULL,
			deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`, userID); err != nil {
		return fmt.Errorf("tombstone: %w", err)
	}

	return tx.Commit()
}

// sendDeletionConfirmation writes to the address the account is losing, while
// it still has one.
//
// It goes out directly rather than through the notification channels: those
// resolve a recipient by user ID and are preference-gated, and neither works
// for an account that is about to stop existing. Without SMTP there is
// nothing to send and nothing to apologise for — an instance can run entirely
// on invite links and passkeys, and this is not the feature that changes it.
func sendDeletionConfirmation(db *database.DB, cfg *config.Config, displayName, username, email string) {
	if cfg == nil || !cfg.SMTP.Configured() || email == "" {
		return
	}

	instance := settings.EffectiveName(db, cfg)
	name := displayName
	if name == "" {
		name = username
	}

	body := fmt.Sprintf(
		"Hello %s,\n\n"+
			"Your account on %s has been deleted, at your own request, just now.\n\n"+
			"What is gone: your email address, display name, bio, links and contact card; "+
			"your passkeys, recovery codes, sessions and calendar link; your notification "+
			"settings; every patch membership you held; and the quilts you had connected "+
			"or followed.\n\n"+
			"What remains: the things you took part in — proposals, votes, comments, "+
			"notices, events and attestations — kept so the community's record stays whole. "+
			"They now show as \"%s\" with no link and no name.\n\n"+
			"Your username stays retired rather than freed, so nobody else can take it and "+
			"inherit links meant for you.\n\n"+
			"Two limits worth knowing. Anything that already federated to other servers is "+
			"held under their policies; %s sends a deletion notice but cannot enforce it. "+
			"And a data export taken before today is a copy nobody here can reach into.\n\n"+
			"If you did not do this, contact the stewards of %s: %s\n",
		name, instance, DeletedAccountName, instance, instance,
		weblink.Absolute(cfg.Instance.Domain, "/label"),
	)

	smtp := cfg.SMTP
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Your %s account has been deleted\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		smtp.From, email, instance, body,
	)
	// Off the request: SMTP is somebody else's server, and somebody deleting
	// their account should not be made to wait on it.
	go func(to string, msg []byte) {
		if err := mail.Send(smtp, []string{to}, msg); err != nil {
			log.Printf("account deletion: confirmation to %s failed: %v", to, err)
		}
	}(email, []byte(msg))
}
