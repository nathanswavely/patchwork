package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Intro emails to the owners of unclaimed patches
// (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md).
//
// A person sends, after seeing the whole message, to an address a person
// typed. Only an instance admin sends, because the message goes out on the
// quilt's own domain; a trusted contributor may enter or suggest the
// address and nothing more. The calendar paragraph is chosen by probing
// the site at preview time and the admin may overrule it. One send per
// patch per address is the rule; a second is an explicit resend.

// IntroSendMail is the relay, injectable for tests the way ClaimSendMail is.
var IntroSendMail func(cfg config.SMTP, to []string, msg []byte) error = mail.Send

// introClaimTokenTTL is how long the claim shortcut in a sent intro stays
// good. Days rather than the claim email's hours: an intro waits in an
// inbox for someone to get round to it, and a dead link in the message
// that introduced the quilt is a poor introduction.
const introClaimTokenTTL = 30 * 24 * time.Hour

// introMaxBytes caps a template part or a sent body.
const introMaxBytes = 32 * 1024

// introTemplate is the shipped text or the instance's replacement for it.
type introTemplate struct {
	Subject  string            `json:"subject"`
	Opener   string            `json:"opener"`
	Calendar map[string]string `json:"calendar"`
	Signoff  string            `json:"signoff"`
}

func defaultIntroTemplate() introTemplate {
	cal := make(map[string]string, len(defaultIntroCalendar))
	for k, v := range defaultIntroCalendar {
		cal[k] = v
	}
	return introTemplate{
		Subject:  defaultIntroSubject,
		Opener:   defaultIntroOpener,
		Calendar: cal,
		Signoff:  defaultIntroSignoff,
	}
}

// effectiveIntroTemplate returns the stored override when one parses, the
// shipped text otherwise. A stored value that no longer parses is treated
// as absent rather than as an outage: the admin sees the default and the
// customized flag off, and can save again.
func effectiveIntroTemplate(db *database.DB) (tpl introTemplate, customized bool, updatedAt string) {
	v, at, ok := settings.GetDetailed(db, settings.KeyOutreachTemplate)
	if !ok || strings.TrimSpace(v) == "" {
		return defaultIntroTemplate(), false, ""
	}
	var stored introTemplate
	if err := json.Unmarshal([]byte(v), &stored); err != nil {
		return defaultIntroTemplate(), false, ""
	}
	// Any part the override leaves empty falls back to the shipped text,
	// so an admin who rewrote the opener did not also blank the sign-off.
	def := defaultIntroTemplate()
	if strings.TrimSpace(stored.Subject) == "" {
		stored.Subject = def.Subject
	}
	if strings.TrimSpace(stored.Opener) == "" {
		stored.Opener = def.Opener
	}
	if strings.TrimSpace(stored.Signoff) == "" {
		stored.Signoff = def.Signoff
	}
	if stored.Calendar == nil {
		stored.Calendar = map[string]string{}
	}
	for k, v := range def.Calendar {
		if strings.TrimSpace(stored.Calendar[k]) == "" {
			stored.Calendar[k] = v
		}
	}
	return stored, true, at
}

// introFacts is everything a template placeholder can name.
type introFacts struct {
	PatchName      string
	PatchURL       string
	ClaimURL       string
	QuiltName      string
	QuiltURL       string
	SenderName     string
	EventCount     int
	AggregatorName string
}

func (f introFacts) replacer() *strings.Replacer {
	return strings.NewReplacer(
		"{patch_name}", f.PatchName,
		"{patch_url}", f.PatchURL,
		"{claim_url}", f.ClaimURL,
		"{quilt_name}", f.QuiltName,
		"{quilt_url}", f.QuiltURL,
		"{sender_name}", f.SenderName,
		"{event_count}", fmt.Sprintf("%d", f.EventCount),
		"{aggregator_name}", f.AggregatorName,
	)
}

// renderIntro fills the template for one situation. Attachable has no
// paragraph of its own (the answer to it is to attach the source), so it
// renders as Other.
func renderIntro(tpl introTemplate, situation eventsource.Situation, f introFacts) (subject, body string) {
	key := string(situation)
	if situation == eventsource.SituationAttachable || !eventsource.ValidSituation(key) {
		key = string(eventsource.SituationOther)
	}
	r := f.replacer()
	parts := []string{
		strings.TrimSpace(r.Replace(tpl.Opener)),
		strings.TrimSpace(r.Replace(tpl.Calendar[key])),
		strings.TrimSpace(r.Replace(tpl.Signoff)),
	}
	return strings.TrimSpace(r.Replace(tpl.Subject)), strings.Join(parts, "\n\n")
}

func quiltURL(cfg *config.Config) string {
	if cfg.Instance.Domain != "" {
		return "https://" + cfg.Instance.Domain
	}
	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}
	return "http://localhost:" + port
}

// introNode is the unclaimed patch an intro is about.
type introNode struct {
	ID                 string
	Name               string
	Slug               string
	Website            string
	ContactEmail       string
	VerificationDomain string
}

func loadIntroNode(db *database.DB, slug string) (introNode, bool) {
	var n introNode
	err := db.QueryRow(
		`SELECT id, name, slug, COALESCE(website,''), contact_email, COALESCE(verification_domain,'')
		 FROM nodes WHERE slug = ? AND status = 'unclaimed' AND removed_at IS NULL`, slug,
	).Scan(&n.ID, &n.Name, &n.Slug, &n.Website, &n.ContactEmail, &n.VerificationDomain)
	return n, err == nil
}

// emailDomain is what follows the last '@' of a normalized address.
func emailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	return email[at+1:]
}

// introSend is one row of intro_sends as the workspace lists it.
type introSend struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	SentBy    string `json:"sent_by"`
	Situation string `json:"situation"`
	Delivery  string `json:"delivery"`
	Subject   string `json:"subject"`
	ClaimLink bool   `json:"claim_link"`
	CreatedAt string `json:"created_at"`
}

func listIntroSends(db *database.DB, nodeID string) []introSend {
	rows, err := db.Query(
		`SELECT s.id, s.email, COALESCE(u.display_name, u.username, ''), s.situation, s.delivery, s.subject,
		        s.claim_token IS NOT NULL, s.created_at
		 FROM intro_sends s LEFT JOIN users u ON u.id = s.sent_by
		 WHERE s.node_id = ? ORDER BY s.created_at DESC`, nodeID,
	)
	if err != nil {
		return []introSend{}
	}
	defer rows.Close()
	out := []introSend{}
	for rows.Next() {
		var s introSend
		if err := rows.Scan(&s.ID, &s.Email, &s.SentBy, &s.Situation, &s.Delivery, &s.Subject, &s.ClaimLink, &s.CreatedAt); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// SetNodeContact handles PUT /api/v1/nodes/{slug}/contact — the address an
// intro goes to. An instance admin, or a trusted contributor whose grant
// reaches this unclaimed patch, may enter it; only the admin may send to
// it. Empty clears it. Changing it revokes any unused claim shortcut a
// sent intro carried, because that token vouched for the old address.
func SetNodeContact(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := loadIntroNode(db, r.PathValue("slug"))
		if !ok {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if user.Role != "admin" && !userTrustedOn(db, user, n.ID) {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		email := ""
		if strings.TrimSpace(req.Email) != "" {
			var err error
			email, err = auth.NormalizeEmail(req.Email)
			if err != nil {
				http.Error(w, `{"error":"a valid email address is required"}`, http.StatusBadRequest)
				return
			}
		}
		now := clock.Now()
		if _, err := db.Exec(`UPDATE nodes SET contact_email = ?, updated_at = ? WHERE id = ?`, email, now, n.ID); err != nil {
			http.Error(w, `{"error":"failed to save contact"}`, http.StatusInternalServerError)
			return
		}
		if email != n.ContactEmail {
			db.Exec(`UPDATE intro_sends SET claim_token = NULL, claim_token_expires_at = NULL
			         WHERE node_id = ? AND claim_token IS NOT NULL AND claim_token_used_at IS NULL`, n.ID)
		}
		auth.LogAuditEventJSON(db, user.ID, "node.contact_set", "node", n.ID, map[string]any{"set": email != ""}, clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"contact_email": email})
	}
}

// IntroState handles GET /api/v1/nodes/{slug}/intro — what the workspace
// section needs: the address, the sends so far, and whether sending is
// possible here at all. Instance admin only.
func IntroState(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, ok := loadIntroNode(db, r.PathValue("slug"))
		if !ok {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"contact_email":       n.ContactEmail,
			"verification_domain": n.VerificationDomain,
			"claim_shortcut":      n.ContactEmail != "" && n.VerificationDomain != "" && emailDomain(n.ContactEmail) == n.VerificationDomain,
			"enabled":             settings.OutreachEnabled(db),
			"smtp":                cfg.SMTP.Configured(),
			"sends":               listIntroSends(db, n.ID),
		})
	}
}

// introFactsFor gathers what the placeholders name.
func introFactsFor(db *database.DB, cfg *config.Config, n introNode, senderID, aggregatorName string) introFacts {
	var sender string
	db.QueryRow(`SELECT COALESCE(NULLIF(display_name,''), username) FROM users WHERE id = ?`, senderID).Scan(&sender)
	var upcoming int
	db.QueryRow(
		`SELECT COUNT(*) FROM events WHERE node_id = ? AND removed_at IS NULL AND status = 'active'
		   AND starts_at >= strftime('%Y-%m-%dT%H:%M:%SZ','now')`, n.ID,
	).Scan(&upcoming)
	base := quiltURL(cfg)
	return introFacts{
		PatchName:      n.Name,
		PatchURL:       base + weblink.Patch(n.Slug),
		ClaimURL:       base + weblink.Patch(n.Slug) + "/claim",
		QuiltName:      settings.EffectiveName(db, cfg),
		QuiltURL:       base,
		SenderName:     sender,
		EventCount:     upcoming,
		AggregatorName: aggregatorName,
	}
}

// IntroPreview handles POST /api/v1/nodes/{slug}/intro/preview — classify
// the patch's calendar and render the whole message for the admin to
// read, edit, and then send. Instance admin only. The fetch this makes
// is the one the ADR allows: on the admin's action, on this request.
func IntroPreview(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := loadIntroNode(db, r.PathValue("slug"))
		if !ok {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		// The admin may ask for a different paragraph than the probe picked.
		var req struct {
			Situation string `json:"situation"`
		}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&req)
		}

		class := eventsource.Classify(r.Context(), db, n.ID, n.Website)
		chosen := class.Situation
		if class.Situation == eventsource.SituationAttachable {
			chosen = eventsource.SituationOther
		}
		if req.Situation != "" {
			if !eventsource.ValidSituation(req.Situation) {
				http.Error(w, `{"error":"unknown situation"}`, http.StatusBadRequest)
				return
			}
			chosen = eventsource.Situation(req.Situation)
		}

		tpl, _, _ := effectiveIntroTemplate(db)
		facts := introFactsFor(db, cfg, n, user.ID, class.AggregatorName)
		subject, body := renderIntro(tpl, chosen, facts)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"classification": class,
			"situation":      chosen,
			"subject":        subject,
			"body":           body,
			"event_count":    facts.EventCount,
			"claim_url":      facts.ClaimURL,
			"claim_shortcut": n.ContactEmail != "" && emailDomain(n.ContactEmail) == n.VerificationDomain && n.VerificationDomain != "",
			"contact_email":  n.ContactEmail,
			"enabled":        settings.OutreachEnabled(db),
			"smtp":           cfg.SMTP.Configured(),
		})
	}
}

// IntroSend handles POST /api/v1/nodes/{slug}/intro/send — send the message
// the admin just read, or record that they sent it elsewhere. Instance
// admin only.
func IntroSend(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := loadIntroNode(db, r.PathValue("slug"))
		if !ok {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !settings.OutreachEnabled(db) {
			http.Error(w, `{"error":"intro emails are turned off on this quilt"}`, http.StatusForbidden)
			return
		}
		if n.ContactEmail == "" {
			http.Error(w, `{"error":"enter a contact address first"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Subject   string `json:"subject"`
			Body      string `json:"body"`
			Situation string `json:"situation"`
			Delivery  string `json:"delivery"`
			Resend    bool   `json:"resend"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.Subject = strings.TrimSpace(req.Subject)
		req.Body = strings.TrimSpace(req.Body)
		if req.Subject == "" || req.Body == "" {
			http.Error(w, `{"error":"the message needs a subject and a body"}`, http.StatusBadRequest)
			return
		}
		if len(req.Body) > introMaxBytes || len(req.Subject) > 500 {
			http.Error(w, `{"error":"the message is too long"}`, http.StatusBadRequest)
			return
		}
		if !eventsource.ValidSituation(req.Situation) {
			http.Error(w, `{"error":"unknown situation"}`, http.StatusBadRequest)
			return
		}
		if req.Delivery == "" {
			req.Delivery = "smtp"
		}
		if req.Delivery != "smtp" && req.Delivery != "copied" {
			http.Error(w, `{"error":"delivery must be smtp or copied"}`, http.StatusBadRequest)
			return
		}
		if req.Delivery == "smtp" && !cfg.SMTP.Configured() {
			http.Error(w, `{"error":"this quilt cannot send email; copy the message and send it yourself"}`, http.StatusBadRequest)
			return
		}

		// One send per patch per address (decision 5). A second one is an
		// explicit resend, never a double click.
		var prior int
		db.QueryRow(`SELECT COUNT(*) FROM intro_sends WHERE node_id = ? AND email = ?`, n.ID, n.ContactEmail).Scan(&prior)
		if prior > 0 && !req.Resend {
			http.Error(w, `{"error":"an intro already went to this address; resend it on purpose"}`, http.StatusConflict)
			return
		}

		// The claim shortcut (decision 6): only to the verification domain,
		// and only when the admin left the claim link in the message.
		facts := introFactsFor(db, cfg, n, user.ID, "")
		body := req.Body
		var claimToken any
		var claimExpiry any
		if n.VerificationDomain != "" && emailDomain(n.ContactEmail) == n.VerificationDomain && strings.Contains(body, facts.ClaimURL) {
			b := make([]byte, 16)
			rand.Read(b)
			tok := hex.EncodeToString(b)
			body = strings.ReplaceAll(body, facts.ClaimURL, facts.ClaimURL+"?intro="+tok)
			claimToken = tok
			claimExpiry = clock.Format(time.Now().Add(introClaimTokenTTL))
		}

		if req.Delivery == "smtp" {
			var replyTo string
			db.QueryRow(`SELECT COALESCE(email,'') FROM users WHERE id = ?`, user.ID).Scan(&replyTo)
			if err := sendIntroMail(cfg, n.ContactEmail, replyTo, req.Subject, body); err != nil {
				http.Error(w, `{"error":"the relay refused the message; nothing was recorded"}`, http.StatusBadGateway)
				return
			}
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO intro_sends (id, node_id, email, sent_by, situation, delivery, subject, body, claim_token, claim_token_expires_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, n.ID, n.ContactEmail, user.ID, req.Situation, req.Delivery, req.Subject, body, claimToken, claimExpiry,
		); err != nil {
			http.Error(w, `{"error":"failed to record the send"}`, http.StatusInternalServerError)
			return
		}
		action := "outreach.send"
		if prior > 0 {
			action = "outreach.resend"
		}
		auth.LogAuditEventJSON(db, user.ID, action, "node", n.ID, map[string]any{
			"situation": req.Situation, "delivery": req.Delivery, "claim_link": claimToken != nil,
		}, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":         id,
			"delivery":   req.Delivery,
			"claim_link": claimToken != nil,
			"sends":      listIntroSends(db, n.ID),
		})
	}
}

// sendIntroMail delivers one plain-text message. Plain text on purpose: it
// is a note from a person, and the notification template's button and
// footer would make it read as the product writing.
func sendIntroMail(cfg *config.Config, to, replyTo, subject, body string) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "From: %s\r\n", cfg.SMTP.From)
	fmt.Fprintf(&sb, "To: %s\r\n", to)
	if replyTo != "" {
		fmt.Fprintf(&sb, "Reply-To: %s\r\n", replyTo)
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", strings.ReplaceAll(strings.ReplaceAll(subject, "\r", ""), "\n", " "))
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\nMIME-Version: 1.0\r\n\r\n")
	sb.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return IntroSendMail(cfg.SMTP, []string{to}, []byte(sb.String()))
}

// consumeIntroToken redeems the claim shortcut a sent intro carried
// (decision 6): the token must belong to this patch, be unused and
// unexpired, and the address it vouched for must still be at the
// patch's verification domain. It returns that address, which becomes
// the email on the claim. Marking it used happens here, before the claim
// is approved, so a token can never approve two claims.
func consumeIntroToken(db *database.DB, nodeID, token string) (email string, ok bool) {
	if token == "" {
		return "", false
	}
	var expires, vd string
	err := db.QueryRow(
		`SELECT s.email, COALESCE(s.claim_token_expires_at,''), COALESCE(n.verification_domain,'')
		 FROM intro_sends s JOIN nodes n ON n.id = s.node_id
		 WHERE s.claim_token = ? AND s.node_id = ? AND s.claim_token_used_at IS NULL`, token, nodeID,
	).Scan(&email, &expires, &vd)
	if err != nil {
		return "", false
	}
	if exp, err := clock.Parse(expires); err != nil || time.Now().UTC().After(exp) {
		return "", false
	}
	if vd == "" || emailDomain(email) != vd {
		return "", false
	}
	res, err := db.Exec(
		`UPDATE intro_sends SET claim_token_used_at = ? WHERE claim_token = ? AND claim_token_used_at IS NULL`,
		clock.Now(), token,
	)
	if err != nil {
		return "", false
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return "", false
	}
	return email, true
}

// AdminOutreach handles GET /api/v1/admin/outreach — the backlog: every
// unclaimed patch with its address and what has gone to it, plus the
// template in effect, so the Outreach page is one read.
func AdminOutreach(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(
			`SELECT n.id, n.name, n.slug, COALESCE(n.website,''), n.contact_email,
			        (SELECT COUNT(*) FROM intro_sends s WHERE s.node_id = n.id),
			        COALESCE((SELECT MAX(s.created_at) FROM intro_sends s WHERE s.node_id = n.id), ''),
			        (SELECT COUNT(*) FROM event_sources es WHERE es.node_id = n.id AND es.aggregator_id IS NULL AND es.status = 'ok') > 0,
			        (SELECT COUNT(*) FROM event_sources es WHERE es.node_id = n.id AND es.aggregator_id IS NOT NULL) > 0,
			        (SELECT COUNT(*) FROM claim_requests c WHERE c.node_id = n.id AND c.status IN ('pending','approved'))
			 FROM nodes n WHERE n.status = 'unclaimed' AND n.removed_at IS NULL
			 ORDER BY (SELECT COUNT(*) FROM intro_sends s WHERE s.node_id = n.id), n.name`,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to list patches"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		patches := []map[string]any{}
		for rows.Next() {
			var id, name, slug, website, email, last string
			var sends, claims int
			var own, agg bool
			if err := rows.Scan(&id, &name, &slug, &website, &email, &sends, &last, &own, &agg, &claims); err != nil {
				continue
			}
			patches = append(patches, map[string]any{
				"id": id, "name": name, "slug": slug, "website": website,
				"contact_email": email, "sends": sends, "last_sent_at": last,
				"own_source": own, "aggregator_source": agg, "open_claims": claims,
			})
		}
		tpl, customized, updatedAt := effectiveIntroTemplate(db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":          settings.OutreachEnabled(db),
			"smtp":             cfg.SMTP.Configured(),
			"patches":          patches,
			"template":         map[string]any{"parts": tpl, "customized": customized, "updated_at": updatedAt},
			"default_template": defaultIntroTemplate(),
			"situations":       eventsource.Situations,
		})
	}
}

// AdminUpdateOutreach handles PATCH /api/v1/admin/outreach — the on/off
// switch and the template, either or both.
func AdminUpdateOutreach(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		var req struct {
			Enabled  *bool          `json:"enabled"`
			Template *introTemplate `json:"template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Enabled != nil {
			v := "true"
			if !*req.Enabled {
				v = "false"
			}
			if err := settings.Set(db, settings.KeyOutreachEnabled, v); err != nil {
				http.Error(w, `{"error":"failed to save setting"}`, http.StatusInternalServerError)
				return
			}
		}
		if req.Template != nil {
			t := *req.Template
			total := len(t.Subject) + len(t.Opener) + len(t.Signoff)
			for k, v := range t.Calendar {
				if !eventsource.ValidSituation(k) {
					http.Error(w, `{"error":"unknown situation in template"}`, http.StatusBadRequest)
					return
				}
				total += len(v)
			}
			if total > introMaxBytes*4 {
				http.Error(w, `{"error":"template is too large"}`, http.StatusBadRequest)
				return
			}
			raw, _ := json.Marshal(t)
			if err := settings.Set(db, settings.KeyOutreachTemplate, string(raw)); err != nil {
				http.Error(w, `{"error":"failed to save template"}`, http.StatusInternalServerError)
				return
			}
		}
		auth.LogAuditEventJSON(db, adminUser.ID, "admin.outreach_update", "instance", "", map[string]any{
			"enabled": req.Enabled, "template": req.Template != nil,
		}, clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminResetOutreachTemplate handles DELETE /api/v1/admin/outreach/template
// — drop the instance's text and return to the shipped default.
func AdminResetOutreachTemplate(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		if err := settings.Unset(db, settings.KeyOutreachTemplate); err != nil {
			http.Error(w, `{"error":"failed to reset template"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, adminUser.ID, "admin.outreach_template_reset", "instance", "", "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
