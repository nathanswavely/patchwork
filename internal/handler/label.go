package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// The Label (docs/adr/023): the quilt's public statement of how it is run
// and paid for. A disclosure about the deployment, not a biography of its
// admin — stewards are named inside it and own their own listing.

// Staleness threshold: past this, the page itself tells every reader the
// figures haven't been reviewed — the disclosure discloses its own
// reliability.
const labelStaleAfter = 180 * 24 * time.Hour

// labelSteward is one person's listing, as the public Label renders it.
type labelSteward struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Blurb       string `json:"blurb"`
}

// labelCostItem is one structured cost line.
type labelCostItem struct {
	ID          string `json:"id,omitempty"`
	Service     string `json:"service"`
	Purpose     string `json:"purpose"`
	Why         string `json:"why"`
	AmountMinor int64  `json:"amount_minor"`
	Period      string `json:"period"` // monthly | yearly
	StatedOn    string `json:"stated_on"`
	Source      string `json:"source"` // 'manual' is first-class (ADR 023)
}

// monthlyMinor normalizes a cost item to minor units per month. Yearly
// items divide by 12; integer division is fine for an explicitly
// approximate total.
func (c labelCostItem) monthlyMinor() int64 {
	if c.Period == "yearly" {
		return c.AmountMinor / 12
	}
	return c.AmountMinor
}

func loadLabelRow(db *database.DB) (prose, supportURL, feedbackURL, currency, fromName, fromURL string, published bool, ok bool) {
	var pub int
	err := db.QueryRow(`SELECT prose, support_url, feedback_url, currency,
			seamripped_from_name, seamripped_from_url, published
		FROM label WHERE id = 1`).
		Scan(&prose, &supportURL, &feedbackURL, &currency, &fromName, &fromURL, &pub)
	if err != nil {
		return "", "", "", "USD", "", "", false, false
	}
	return prose, supportURL, feedbackURL, currency, fromName, fromURL, pub != 0, true
}

func loadListedStewards(db *database.DB) []labelSteward {
	stewards := []labelSteward{}
	rows, err := db.Query(`SELECT u.username, u.display_name, u.avatar_url, s.blurb
		FROM label_stewards s JOIN users u ON u.id = s.user_id
		WHERE s.listed = 1 ORDER BY s.position, s.created_at`)
	if err != nil {
		return stewards
	}
	defer rows.Close()
	for rows.Next() {
		var s labelSteward
		if rows.Scan(&s.Username, &s.DisplayName, &s.AvatarURL, &s.Blurb) == nil {
			stewards = append(stewards, s)
		}
	}
	return stewards
}

func loadCostItems(db *database.DB) []labelCostItem {
	items := []labelCostItem{}
	rows, err := db.Query(`SELECT id, service, purpose, why, amount_minor, period, stated_on, source
		FROM label_cost_items ORDER BY position, service`)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var c labelCostItem
		if rows.Scan(&c.ID, &c.Service, &c.Purpose, &c.Why, &c.AmountMinor, &c.Period, &c.StatedOn, &c.Source) == nil {
			items = append(items, c)
		}
	}
	return items
}

// costSummary computes the monthly total and the oldest stated_on date.
// A stale money claim is worse than none — the oldest date is the one
// the staleness banner hangs on.
func costSummary(items []labelCostItem) (totalMonthlyMinor int64, oldestStatedOn string, stale bool) {
	for _, c := range items {
		totalMonthlyMinor += c.monthlyMinor()
		if oldestStatedOn == "" || c.StatedOn < oldestStatedOn {
			oldestStatedOn = c.StatedOn
		}
	}
	if oldestStatedOn != "" {
		if t, err := time.Parse("2006-01-02", oldestStatedOn); err == nil {
			stale = time.Since(t) > labelStaleAfter
		}
	}
	return
}

// LabelSummary is the cross-quilt-safe subset exposed on
// GET /api/v1/instance: steward *count*, never handles (docs/adr/023 —
// consenting to a page is not consenting to a scrapeable directory).
type LabelSummary struct {
	Published         bool   `json:"published"`
	URL               string `json:"url"`
	StewardCount      int    `json:"steward_count"`
	TotalMonthlyMinor int64  `json:"total_monthly_minor"`
	Currency          string `json:"currency"`
	StatedOn          string `json:"stated_on,omitempty"`
	Stale             bool   `json:"stale"`
}

func labelSummary(db *database.DB) LabelSummary {
	_, _, _, currency, _, _, published, ok := loadLabelRow(db)
	if !ok || !published {
		return LabelSummary{Published: false, URL: "/label"}
	}
	items := loadCostItems(db)
	total, oldest, stale := costSummary(items)
	return LabelSummary{
		Published:         true,
		URL:               "/label",
		StewardCount:      len(loadListedStewards(db)),
		TotalMonthlyMinor: total,
		Currency:          currency,
		StatedOn:          oldest,
		Stale:             stale,
	}
}

// GetLabel handles GET /api/v1/label — public, readable logged out: the
// Label's most important reader has no account yet.
func GetLabel(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prose, supportURL, feedbackURL, currency, fromName, fromURL, published, ok := loadLabelRow(db)
		w.Header().Set("Content-Type", "application/json")
		if !ok || !published {
			json.NewEncoder(w).Encode(map[string]any{"published": false})
			return
		}
		items := loadCostItems(db)
		total, oldest, stale := costSummary(items)
		json.NewEncoder(w).Encode(map[string]any{
			"published":            true,
			"prose":                prose,
			"support_url":          supportURL,
			"feedback_url":         feedbackURL,
			"currency":             currency,
			"seamripped_from_name": fromName,
			"seamripped_from_url":  fromURL,
			"stewards":             loadListedStewards(db),
			"cost_items":           items,
			"total_monthly_minor":  total,
			"stated_on":            oldest,
			"stale":                stale,
		})
	}
}

// validateHTTPURL accepts empty (unset) or an absolute http(s) URL.
func validateHTTPURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// enforceStewardFloor unpublishes the Label if no listed steward remains.
// The floor of one holds at all times, not just at publish: a Label run
// by nobody is the unaccountable posture the feature refuses.
func enforceStewardFloor(db *database.DB) {
	var listed int
	db.QueryRow(`SELECT COUNT(*) FROM label_stewards WHERE listed = 1`).Scan(&listed)
	if listed == 0 {
		db.Exec(`UPDATE label SET published = 0,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = 1`)
	}
}

// AdminGetLabel handles GET /api/v1/admin/label — the full editing state,
// including unlisted (invited) stewards, which the public endpoint never
// shows.
func AdminGetLabel(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prose, supportURL, feedbackURL, currency, fromName, fromURL, published, _ := loadLabelRow(db)

		type adminSteward struct {
			ID          string `json:"id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Blurb       string `json:"blurb"`
			Listed      bool   `json:"listed"`
		}
		stewards := []adminSteward{}
		rows, err := db.Query(`SELECT s.id, u.username, u.display_name, s.blurb, s.listed
			FROM label_stewards s JOIN users u ON u.id = s.user_id
			ORDER BY s.position, s.created_at`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var s adminSteward
				var listed int
				if rows.Scan(&s.ID, &s.Username, &s.DisplayName, &s.Blurb, &listed) == nil {
					s.Listed = listed != 0
					stewards = append(stewards, s)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"published":            published,
			"prose":                prose,
			"support_url":          supportURL,
			"feedback_url":         feedbackURL,
			"currency":             currency,
			"seamripped_from_name": fromName,
			"seamripped_from_url":  fromURL,
			"stewards":             stewards,
			"cost_items":           loadCostItems(db),
		})
	}
}

// AdminUpdateLabel handles PATCH /api/v1/admin/label.
func AdminUpdateLabel(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			Prose              *string `json:"prose"`
			SupportURL         *string `json:"support_url"`
			FeedbackURL        *string `json:"feedback_url"`
			Currency           *string `json:"currency"`
			SeamrippedFromName *string `json:"seamripped_from_name"`
			SeamrippedFromURL  *string `json:"seamripped_from_url"`
			Published          *bool   `json:"published"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Ensure the single row exists before piecewise updates.
		if _, err := db.Exec(`INSERT OR IGNORE INTO label (id) VALUES (1)`); err != nil {
			http.Error(w, `{"error":"failed to save label"}`, http.StatusInternalServerError)
			return
		}

		set := func(column, value string) bool {
			_, err := db.Exec(`UPDATE label SET `+column+` = ?,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = 1`, value)
			if err != nil {
				http.Error(w, `{"error":"failed to save label"}`, http.StatusInternalServerError)
				return false
			}
			return true
		}

		if req.Prose != nil {
			if len(*req.Prose) > 20000 {
				http.Error(w, `{"error":"prose must be 20000 characters or fewer"}`, http.StatusBadRequest)
				return
			}
			if !set("prose", *req.Prose) {
				return
			}
		}
		if req.SupportURL != nil {
			if !validateHTTPURL(strings.TrimSpace(*req.SupportURL)) {
				http.Error(w, `{"error":"support link must be an http(s) URL"}`, http.StatusBadRequest)
				return
			}
			if !set("support_url", strings.TrimSpace(*req.SupportURL)) {
				return
			}
		}
		if req.FeedbackURL != nil {
			if !validateHTTPURL(strings.TrimSpace(*req.FeedbackURL)) {
				http.Error(w, `{"error":"feedback link must be an http(s) URL"}`, http.StatusBadRequest)
				return
			}
			if !set("feedback_url", strings.TrimSpace(*req.FeedbackURL)) {
				return
			}
		}
		if req.Currency != nil {
			cur := strings.ToUpper(strings.TrimSpace(*req.Currency))
			if len(cur) != 3 {
				http.Error(w, `{"error":"currency must be a 3-letter code"}`, http.StatusBadRequest)
				return
			}
			if !set("currency", cur) {
				return
			}
		}
		// The provenance line is removable without ceremony (docs/adr/023):
		// a community fleeing its stewards is never forced to keep a link
		// to them. Setting both to "" is that removal.
		if req.SeamrippedFromName != nil {
			if !set("seamripped_from_name", strings.TrimSpace(*req.SeamrippedFromName)) {
				return
			}
		}
		if req.SeamrippedFromURL != nil {
			if !validateHTTPURL(strings.TrimSpace(*req.SeamrippedFromURL)) {
				http.Error(w, `{"error":"seamripped-from link must be an http(s) URL"}`, http.StatusBadRequest)
				return
			}
			if !set("seamripped_from_url", strings.TrimSpace(*req.SeamrippedFromURL)) {
				return
			}
		}

		if req.Published != nil {
			if *req.Published {
				// The floor: a Label cannot publish with zero listed
				// stewards. The buck stops somewhere.
				var listed int
				db.QueryRow(`SELECT COUNT(*) FROM label_stewards WHERE listed = 1`).Scan(&listed)
				if listed == 0 {
					http.Error(w, `{"error":"the Label needs at least one listed steward before it can publish"}`, http.StatusBadRequest)
					return
				}
				if _, err := db.Exec(`UPDATE label SET published = 1,
					updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = 1`); err != nil {
					http.Error(w, `{"error":"failed to publish"}`, http.StatusInternalServerError)
					return
				}
			} else {
				if _, err := db.Exec(`UPDATE label SET published = 0,
					updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = 1`); err != nil {
					http.Error(w, `{"error":"failed to unpublish"}`, http.StatusInternalServerError)
					return
				}
			}
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.label_update", "instance", "", "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminPutLabelCosts handles PUT /api/v1/admin/label/costs — replaces the
// whole cost list. Community-scale data; item CRUD would be ceremony.
func AdminPutLabelCosts(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			Items []labelCostItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if len(req.Items) > 50 {
			http.Error(w, `{"error":"too many cost items"}`, http.StatusBadRequest)
			return
		}
		for i := range req.Items {
			c := &req.Items[i]
			c.Service = strings.TrimSpace(c.Service)
			if c.Service == "" {
				http.Error(w, `{"error":"every cost item needs a service name"}`, http.StatusBadRequest)
				return
			}
			if c.AmountMinor < 0 {
				http.Error(w, `{"error":"amounts cannot be negative"}`, http.StatusBadRequest)
				return
			}
			if c.Period != "monthly" && c.Period != "yearly" {
				http.Error(w, `{"error":"period must be monthly or yearly"}`, http.StatusBadRequest)
				return
			}
			if _, err := time.Parse("2006-01-02", c.StatedOn); err != nil {
				http.Error(w, `{"error":"stated_on must be a YYYY-MM-DD date"}`, http.StatusBadRequest)
				return
			}
			// Only the manual source ships (docs/adr/023); the column is
			// the hook for future resource-bound providers.
			c.Source = "manual"
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"failed to save costs"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM label_cost_items`); err != nil {
			http.Error(w, `{"error":"failed to save costs"}`, http.StatusInternalServerError)
			return
		}
		for i, c := range req.Items {
			if _, err := tx.Exec(`INSERT INTO label_cost_items
				(id, service, purpose, why, amount_minor, period, stated_on, source, position)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				auth.NewUUIDv7(), c.Service, c.Purpose, c.Why, c.AmountMinor, c.Period, c.StatedOn, c.Source, i); err != nil {
				http.Error(w, `{"error":"failed to save costs"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"failed to save costs"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.label_costs_update", "instance", "", "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminAddLabelSteward handles POST /api/v1/admin/label/stewards.
// Adding yourself lists you immediately — you are consenting in the act.
// Adding anyone else creates an unlisted invitation only that person can
// accept: consent to appear is given by the person appearing.
func AdminAddLabelSteward(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		var userID string
		err := db.QueryRow(`SELECT id FROM users WHERE username = ?`,
			strings.ToLower(strings.TrimSpace(req.Username))).Scan(&userID)
		if err != nil {
			http.Error(w, `{"error":"no such user"}`, http.StatusNotFound)
			return
		}

		listed := 0
		if userID == adminUser.ID {
			listed = 1
		}
		var position int
		db.QueryRow(`SELECT COALESCE(MAX(position)+1, 0) FROM label_stewards`).Scan(&position)
		if _, err := db.Exec(`INSERT INTO label_stewards (id, user_id, listed, position)
			VALUES (?, ?, ?, ?)`, auth.NewUUIDv7(), userID, listed, position); err != nil {
			http.Error(w, `{"error":"that person is already on the stewards list"}`, http.StatusConflict)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.label_steward_add", "user", userID, "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminRemoveLabelSteward handles DELETE /api/v1/admin/label/stewards/{id}.
func AdminRemoveLabelSteward(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		id := r.PathValue("id")
		res, err := db.Exec(`DELETE FROM label_stewards WHERE id = ?`, id)
		if err != nil {
			http.Error(w, `{"error":"failed to remove steward"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, `{"error":"no such steward"}`, http.StatusNotFound)
			return
		}
		enforceStewardFloor(db)
		auth.LogAuditEvent(db, adminUser.ID, "admin.label_steward_remove", "steward", id, "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// GetMyStewardListing handles GET /api/v1/users/me/steward — whether the
// signed-in person is on the stewards list, and in what state. Powers the
// invitation card in account settings.
func GetMyStewardListing(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		var blurb string
		var listed int
		err := db.QueryRow(`SELECT blurb, listed FROM label_stewards WHERE user_id = ?`,
			user.ID).Scan(&blurb, &listed)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"steward": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"steward": true,
			"listed":  listed != 0,
			"blurb":   blurb,
		})
	}
}

// UpdateMyStewardListing handles PATCH /api/v1/users/me/steward — the
// person's own switch (docs/adr/023, in the spirit of ADR 006). Only the
// person appearing can list themselves or write their blurb.
func UpdateMyStewardListing(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Listed *bool   `json:"listed"`
			Blurb  *string `json:"blurb"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		var exists int
		if db.QueryRow(`SELECT COUNT(*) FROM label_stewards WHERE user_id = ?`, user.ID).Scan(&exists); exists == 0 {
			http.Error(w, `{"error":"you are not on the stewards list"}`, http.StatusNotFound)
			return
		}

		if req.Blurb != nil {
			blurb := strings.TrimSpace(*req.Blurb)
			if len(blurb) > 200 {
				http.Error(w, `{"error":"blurb must be 200 characters or fewer"}`, http.StatusBadRequest)
				return
			}
			if _, err := db.Exec(`UPDATE label_stewards SET blurb = ? WHERE user_id = ?`, blurb, user.ID); err != nil {
				http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
				return
			}
		}
		if req.Listed != nil {
			v := 0
			if *req.Listed {
				v = 1
			}
			if _, err := db.Exec(`UPDATE label_stewards SET listed = ? WHERE user_id = ?`, v, user.ID); err != nil {
				http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
				return
			}
			// Unlisting the last listed steward unpublishes the Label:
			// the floor holds at all times.
			enforceStewardFloor(db)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// DeleteMyStewardListing handles DELETE /api/v1/users/me/steward —
// declining an invitation or stepping down entirely.
func DeleteMyStewardListing(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		if _, err := db.Exec(`DELETE FROM label_stewards WHERE user_id = ?`, user.ID); err != nil {
			http.Error(w, `{"error":"failed to remove listing"}`, http.StatusInternalServerError)
			return
		}
		enforceStewardFloor(db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
