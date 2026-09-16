package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// What a rule costs this patch today (docs/adr/104).
//
// The rules editor asks for a quorum as a bare percentage and a tenure bar as
// a bare number of days, and says nothing about what either comes to for the
// patch being edited. A simulated eight-person co-op took the Formal defaults
// — consensus, 50% quorum, thirty days' tenure — ran three proposals in its
// first month and carried none. Its founder read "Quorum (%)" beside a number
// box and had no way to learn that it meant four of her eight members had to
// act inside a fortnight, or that one reject would defeat anything the other
// seven approved.
//
// The arithmetic is not hard. It is just nowhere. This endpoint is the facts
// the editor needs to do it in front of the person while they choose, without
// a round trip per keystroke: how many people are in the room, how long each
// of them has been there, and how old the patch itself is.
//
// The tenures come as bare numbers, sorted, with no names attached — the
// question is "how many clear thirty days", and answering it does not need to
// say who. It is still a members-and-admins endpoint: the count of a patch's
// members is public (the quilt sizes a tile by it), and when its people
// arrived is a fact about the room.

// electorateFacts is the patch as the rules editor needs to reason about it.
type electorateFacts struct {
	// Members is everyone the electorate is drawn from: active admins and
	// members, before any tenure bar. A follower is not one (docs/adr/044).
	Members int `json:"members"`
	// TenureDays is how long each of those people has been here, in whole
	// days, longest first. `count(t >= bar)` is the electorate a bar of `bar`
	// days would leave — the same comparison `electorateFilter` makes in SQL.
	TenureDays []int `json:"tenure_days"`
	// PatchAgeDays is how old the patch is, because a patch younger than its
	// own bar has no bar (docs/adr/098) and an editor that did not know that
	// would tell a week-old co-op it had an electorate of nobody.
	PatchAgeDays int `json:"patch_age_days"`
}

// GovernanceElectorate handles GET /api/v1/nodes/{slug}/governance/electorate.
func GovernanceElectorate(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		// The room, which is the same set that may raise a rules proposal in
		// the first place. A 403 rather than a 404: the patch is public and
		// so are its rules; what is inside is who is in it.
		if user == nil || !mayPropose(db, user.ID, nodeID) {
			http.Error(w, `{"error":"only this patch's members can see who votes here"}`, http.StatusForbidden)
			return
		}

		facts := electorateFacts{TenureDays: []int{}}
		rows, err := db.Query(
			`SELECT CAST(julianday('now') - julianday(joined_at) AS INTEGER)
			 FROM memberships WHERE node_id = ? AND `+electorateMembership(""), nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to read the electorate"}`, http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var days int
			if rows.Scan(&days) != nil {
				continue
			}
			if days < 0 {
				days = 0
			}
			facts.TenureDays = append(facts.TenureDays, days)
		}
		rows.Close()
		facts.Members = len(facts.TenureDays)
		sort.Sort(sort.Reverse(sort.IntSlice(facts.TenureDays)))

		// An unreadable age reads as brand new, exactly as the tenure cap
		// treats it, so the editor and the gate agree about a patch neither
		// can date.
		facts.PatchAgeDays, _ = patchAgeDays(db, nodeID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(facts)
	}
}
