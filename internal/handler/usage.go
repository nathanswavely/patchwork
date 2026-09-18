package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// The admin panel's Usage tab
// (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md): the
// daily totals the counter kept, and a few counts of what the community did
// in the same window drawn from tables that already exist. Nothing here is
// about a person. The window is days ending today, UTC, because the counter
// rolls its salt at midnight UTC and a day means the same thing in both.

// usageDay is one day's line on the chart. Days with nothing counted are
// present with zeros, so a quiet week draws as a quiet week and not as a
// gap.
type usageDay struct {
	Day      string `json:"day"`
	Views    int    `json:"views"`
	Visitors int    `json:"visitors"`
}

type usagePath struct {
	Path  string `json:"path"`
	Views int    `json:"views"`
}

// usageActivity counts what the community did in the window. These come
// from the tables the site keeps anyway, so they are the same whether
// counting is on or off, and they say what a page-view total cannot:
// whether anyone did anything.
type usageActivity struct {
	Accounts  int `json:"accounts"`
	Joins     int `json:"joins"`
	Follows   int `json:"follows"`
	Events    int `json:"events"`
	Proposals int `json:"proposals"`
}

type usageResponse struct {
	Enabled bool `json:"enabled"`
	// Days is the window that was asked for, after clamping.
	Days int `json:"days"`
	// Since is the first day in the window, for the heading.
	Since  string      `json:"since"`
	Series []usageDay  `json:"series"`
	Paths  []usagePath `json:"paths"`
	// Views is the sum over the window. Visitors is deliberately not summed:
	// a visitor is distinct within one day only, so the sum of daily counts
	// is a number that means nothing. PeakVisitors is the busiest day.
	Views        int           `json:"views"`
	PeakVisitors int           `json:"peak_visitors"`
	Activity     usageActivity `json:"activity"`
	// RetentionMonths is how long a daily row lives before the counter
	// deletes it, stated so the page can say so.
	RetentionMonths int `json:"retention_months"`
}

// usageWindows are the ranges the tab offers. Anything else clamps to the
// default.
var usageWindows = map[int]bool{7: true, 30: true, 90: true, 365: true}

// AdminUsage handles GET /api/v1/admin/usage?days=30.
func AdminUsage(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if !usageWindows[days] {
			days = 30
		}
		today := time.Now().UTC()
		since := today.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

		resp := usageResponse{
			Enabled:         settings.UsageStatsEnabled(db),
			Days:            days,
			Since:           since,
			Series:          []usageDay{},
			Paths:           []usagePath{},
			RetentionMonths: 13,
		}

		views := map[string]int{}
		rows, err := db.Query(`SELECT day, SUM(views) FROM usage_days WHERE day >= ? GROUP BY day`, since)
		if err == nil {
			for rows.Next() {
				var d string
				var n int
				if rows.Scan(&d, &n) == nil {
					views[d] = n
				}
			}
			rows.Close()
		}
		visitors := map[string]int{}
		rows, err = db.Query(`SELECT day, visitors FROM usage_visitors WHERE day >= ?`, since)
		if err == nil {
			for rows.Next() {
				var d string
				var n int
				if rows.Scan(&d, &n) == nil {
					visitors[d] = n
				}
			}
			rows.Close()
		}
		for i := days - 1; i >= 0; i-- {
			d := today.AddDate(0, 0, -i).Format("2006-01-02")
			day := usageDay{Day: d, Views: views[d], Visitors: visitors[d]}
			resp.Views += day.Views
			if day.Visitors > resp.PeakVisitors {
				resp.PeakVisitors = day.Visitors
			}
			resp.Series = append(resp.Series, day)
		}

		rows, err = db.Query(`SELECT path, SUM(views) AS n FROM usage_days WHERE day >= ?
			GROUP BY path ORDER BY n DESC, path LIMIT 25`, since)
		if err == nil {
			for rows.Next() {
				var p usagePath
				if rows.Scan(&p.Path, &p.Views) == nil {
					resp.Paths = append(resp.Paths, p)
				}
			}
			rows.Close()
		}

		// Activity: rows created on or after the window's first instant.
		// Timestamps are ISO 8601 text, so a date prefix compares correctly.
		sinceTS := since + "T00:00:00Z"
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE created_at >= ? AND deleted_at IS NULL`, sinceTS).Scan(&resp.Activity.Accounts)
		db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE joined_at >= ? AND status = 'active' AND role IN ('member', 'admin')`, sinceTS).Scan(&resp.Activity.Joins)
		db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE joined_at >= ? AND status = 'active' AND role = 'follower'`, sinceTS).Scan(&resp.Activity.Follows)
		db.QueryRow(`SELECT COUNT(*) FROM events WHERE created_at >= ?`, sinceTS).Scan(&resp.Activity.Events)
		db.QueryRow(`SELECT COUNT(*) FROM proposals WHERE created_at >= ?`, sinceTS).Scan(&resp.Activity.Proposals)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// AdminClearUsage handles DELETE /api/v1/admin/usage: every daily row goes,
// and so does whatever the counter holds in memory, so the next flush does
// not write the cleared day back. Audited, because it is a deletion.
func AdminClearUsage(db *database.DB, counter *middleware.UsageCounter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		if counter != nil {
			counter.Reset()
		}
		for _, table := range []string{"usage_days", "usage_visitors"} {
			if _, err := db.Exec(`DELETE FROM ` + table); err != nil {
				http.Error(w, `{"error":"failed to clear usage counts"}`, http.StatusInternalServerError)
				return
			}
		}
		auth.LogAuditEvent(db, adminUser.ID, "admin.usage_cleared", "instance", "", "{}", clientIP(r))
		w.WriteHeader(http.StatusNoContent)
	}
}
