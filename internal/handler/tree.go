package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// TreeNode is a representation of a node for the quilt visualization.
type TreeNode struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug,omitempty"`
	Description string          `json:"description,omitempty"`
	Tags        []string        `json:"tags"`
	Appearance  json.RawMessage `json:"appearance,omitempty"`
	Latitude    *float64        `json:"latitude,omitempty"`
	Longitude   *float64        `json:"longitude,omitempty"`
	// MemberCount is admins + members only — followers are counted
	// separately. A follower is an interested observer, not a member
	// (CLAUDE.md roles); public counts must not conflate the two,
	// especially on unclaimed patches where membership is impossible.
	MemberCount   int        `json:"member_count"`
	FollowerCount int        `json:"follower_count"`
	EventCount    int        `json:"event_count"`
	IsUnclaimed   bool       `json:"is_unclaimed,omitempty"`
	Children      []TreeNode `json:"children"`
}

// AffinityLink represents a weighted connection between two patches.
// Strength is float64 because the tag term is fractional (mass-scaled,
// capped below one shared member — docs/adr/021); people terms stay whole.
type AffinityLink struct {
	Source   string  `json:"source"`
	Target   string  `json:"target"`
	Strength float64 `json:"strength"`
}

// NodeTree handles GET /api/v1/nodes/tree.
// Returns all public patches with affinity data for the quilt layout engine.
func NodeTree(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		user := middleware.UserFromContext(r.Context())

		var query string
		var args []interface{}

		if scope == "my" && user != nil {
			// Scoped to user's patches only (any active membership).
			query = `
				SELECT
					n.id, n.name, n.slug, n.description, COALESCE(n.appearance,''), n.status, n.latitude, n.longitude,
					COALESCE((SELECT COUNT(*) FROM memberships m WHERE m.node_id = n.id AND m.status = 'active' AND m.role IN ('admin','member')), 0) AS member_count,
					COALESCE((SELECT COUNT(*) FROM memberships m WHERE m.node_id = n.id AND m.status = 'active' AND m.role = 'follower'), 0) AS follower_count,
					COALESCE((SELECT COUNT(*) FROM events e WHERE e.node_id = n.id AND e.status = 'active'), 0)
					+ COALESCE((SELECT COUNT(*) FROM event_links el JOIN events e ON e.id = el.event_id WHERE el.node_id = n.id AND el.status = 'confirmed' AND e.status = 'active'), 0) AS event_count
				FROM nodes n
				JOIN memberships mem ON mem.node_id = n.id AND mem.user_id = ? AND mem.status = 'active'
				WHERE n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				ORDER BY n.name ASC`
			args = append(args, user.ID)
		} else {
			// All public patches (default).
			query = `
				SELECT
					n.id, n.name, n.slug, n.description, COALESCE(n.appearance,''), n.status, n.latitude, n.longitude,
					COALESCE((SELECT COUNT(*) FROM memberships m WHERE m.node_id = n.id AND m.status = 'active' AND m.role IN ('admin','member')), 0) AS member_count,
					COALESCE((SELECT COUNT(*) FROM memberships m WHERE m.node_id = n.id AND m.status = 'active' AND m.role = 'follower'), 0) AS follower_count,
					COALESCE((SELECT COUNT(*) FROM events e WHERE e.node_id = n.id AND e.status = 'active'), 0)
					+ COALESCE((SELECT COUNT(*) FROM event_links el JOIN events e ON e.id = el.event_id WHERE el.node_id = n.id AND el.status = 'confirmed' AND e.status = 'active'), 0) AS event_count
				FROM nodes n
				WHERE n.status IN ('active','unclaimed') AND n.removed_at IS NULL AND n.visibility = 'public'
				ORDER BY n.name ASC`
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to query nodes"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type flatNode struct {
			ID            string
			Name          string
			Slug          string
			Description   string
			Appearance    string
			Status        string
			Latitude      *float64
			Longitude     *float64
			MemberCount   int
			FollowerCount int
			EventCount    int
		}

		var flat []flatNode
		for rows.Next() {
			var fn flatNode
			if err := rows.Scan(&fn.ID, &fn.Name, &fn.Slug, &fn.Description, &fn.Appearance, &fn.Status, &fn.Latitude, &fn.Longitude, &fn.MemberCount, &fn.FollowerCount, &fn.EventCount); err != nil {
				continue
			}
			flat = append(flat, fn)
		}

		// Fetch tags for all nodes, in stored (priority) order — the first
		// motif-bearing tag derives the motif, so order must be stable.
		tagMap := make(map[string][]string)
		tagRows, err := db.Query(`SELECT nt.node_id, t.name FROM node_tags nt JOIN tags t ON nt.tag_id = t.id ORDER BY nt.node_id, COALESCE(nt.position, 1000000), t.name`)
		if err == nil {
			defer tagRows.Close()
			for tagRows.Next() {
				var nodeID, tagName string
				if tagRows.Scan(&nodeID, &tagName) == nil {
					tagMap[nodeID] = append(tagMap[nodeID], tagName)
				}
			}
		}

		// Compute affinity between all patch pairs.
		// Weighted: shared admins/members (weight 3), shared followers (weight 1),
		// shared events (weight 2), plus a fractional shared-tag term (below).
		// These are placement affinity, not threads — threads (the user-facing
		// connection concept) are member-only. See docs/adr/021 and CLAUDE.md.
		affinityMap := make(map[string]map[string]float64)

		// Shared admins/members (weight 3 per shared person).
		memberRows, err := db.Query(`
			SELECT m1.node_id, m2.node_id, COUNT(*) * 3 AS score
			FROM memberships m1
			JOIN memberships m2 ON m1.user_id = m2.user_id
				AND m1.node_id < m2.node_id
				AND m1.status = 'active' AND m2.status = 'active'
				AND m1.role IN ('admin', 'member') AND m2.role IN ('admin', 'member')
			GROUP BY m1.node_id, m2.node_id
		`)
		if err == nil {
			defer memberRows.Close()
			for memberRows.Next() {
				var a, b string
				var score int
				if memberRows.Scan(&a, &b, &score) == nil {
					addAffinity(affinityMap, a, b, float64(score))
				}
			}
		}

		// Shared followers (weight 1 per shared follower).
		followerRows, err := db.Query(`
			SELECT m1.node_id, m2.node_id, COUNT(*) AS score
			FROM memberships m1
			JOIN memberships m2 ON m1.user_id = m2.user_id
				AND m1.node_id < m2.node_id
				AND m1.status = 'active' AND m2.status = 'active'
				AND m1.role = 'follower' AND m2.role = 'follower'
			GROUP BY m1.node_id, m2.node_id
		`)
		if err == nil {
			defer followerRows.Close()
			for followerRows.Next() {
				var a, b string
				var score int
				if followerRows.Scan(&a, &b, &score) == nil {
					addAffinity(affinityMap, a, b, float64(score))
				}
			}
		}

		// Shared event participation: patches whose members attend events at the other patch (weight 2).
		// A user who is a member of patch A and has created an event at patch B creates affinity.
		eventRows, err := db.Query(`
			SELECT m.node_id, e.node_id, COUNT(DISTINCT m.user_id) * 2 AS score
			FROM memberships m
			JOIN events e ON m.user_id = e.created_by
				AND m.node_id != e.node_id
				AND m.status = 'active'
				AND m.role IN ('admin', 'member')
			JOIN nodes n1 ON m.node_id = n1.id AND n1.status IN ('active','unclaimed') AND n1.visibility = 'public'
			JOIN nodes n2 ON e.node_id = n2.id AND n2.status IN ('active','unclaimed') AND n2.visibility = 'public'
			GROUP BY m.node_id, e.node_id
		`)
		if err == nil {
			defer eventRows.Close()
			for eventRows.Next() {
				var a, b string
				var score int
				if eventRows.Scan(&a, &b, &score) == nil {
					// Normalize direction so a < b.
					if a > b {
						a, b = b, a
					}
					addAffinity(affinityMap, a, b, float64(score))
				}
			}
		}

		// Confirmed event links (weight 2 per linked event): two patches
		// explicitly agreeing they did a thing together is the strongest
		// event signal we have (docs/adr/032). Same tier as the
		// created_by heuristic above, which it will eventually subsume.
		linkRows, err := db.Query(`
			SELECT e.node_id, el.node_id, COUNT(*) * 2 AS score
			FROM event_links el
			JOIN events e ON e.id = el.event_id AND e.status = 'active'
			JOIN nodes n1 ON e.node_id = n1.id AND n1.status IN ('active','unclaimed') AND n1.visibility = 'public'
			JOIN nodes n2 ON el.node_id = n2.id AND n2.status IN ('active','unclaimed') AND n2.visibility = 'public'
			WHERE el.status = 'confirmed'
			GROUP BY e.node_id, el.node_id
		`)
		if err == nil {
			defer linkRows.Close()
			for linkRows.Next() {
				var a, b string
				var score int
				if linkRows.Scan(&a, &b, &score) == nil {
					if a > b {
						a, b = b, a
					}
					addAffinity(affinityMap, a, b, float64(score))
				}
			}
		}

		// Shared-tag term (docs/adr/021): declared similarity as a weak
		// attractor, so brand-new patches with no people-overlap still land
		// near their kind. Computed in Go over the visible nodes only.
		//
		//   strength = min(sharedTags, 2) × (0.4 + (1+largerMembers)/(1+maxMembers))
		//
		// Max is 2 × 1.4 = 2.8 — always below one shared member (3). The mass
		// factor pulls thin patches harder toward the biggest patch sharing
		// their tags ("gravitation"); on an instance where nothing has members
		// yet, the factor is 1 for every pair and tags attract uniformly.
		maxMembers := 0
		for _, fn := range flat {
			if fn.MemberCount > maxMembers {
				maxMembers = fn.MemberCount
			}
		}
		for i := 0; i < len(flat); i++ {
			iTags := tagMap[flat[i].ID]
			if len(iTags) == 0 {
				continue
			}
			iSet := make(map[string]bool, len(iTags))
			for _, t := range iTags {
				iSet[t] = true
			}
			for j := i + 1; j < len(flat); j++ {
				shared := 0
				for _, t := range tagMap[flat[j].ID] {
					if iSet[t] {
						shared++
					}
				}
				if shared == 0 {
					continue
				}
				base := 2.0
				if shared < 2 {
					base = 1.0
				}
				larger := flat[i].MemberCount
				if flat[j].MemberCount > larger {
					larger = flat[j].MemberCount
				}
				factor := float64(1+larger) / float64(1+maxMembers)
				addAffinity(affinityMap, flat[i].ID, flat[j].ID, base*(0.4+factor))
			}
		}

		// Build affinity links for the response.
		var links []AffinityLink
		seen := make(map[string]bool)
		for a, targets := range affinityMap {
			for b, strength := range targets {
				key := a + ":" + b
				if a > b {
					key = b + ":" + a
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				links = append(links, AffinityLink{Source: a, Target: b, Strength: strength})
			}
		}
		if links == nil {
			links = []AffinityLink{}
		}

		// Build tree nodes (keep API order — frontend handles placement).
		children := make([]TreeNode, 0, len(flat))
		for _, fn := range flat {
			tags := tagMap[fn.ID]
			if tags == nil {
				tags = []string{}
			}
			// Pass appearance through as raw JSON — but only if it parses,
			// since one bad row would otherwise fail the whole encode.
			var appearance json.RawMessage
			if fn.Appearance != "" && json.Valid([]byte(fn.Appearance)) {
				appearance = json.RawMessage(fn.Appearance)
			}
			children = append(children, TreeNode{
				ID:            fn.ID,
				Name:          fn.Name,
				Slug:          fn.Slug,
				Description:   fn.Description,
				Tags:          tags,
				Appearance:    appearance,
				Latitude:      fn.Latitude,
				Longitude:     fn.Longitude,
				MemberCount:   fn.MemberCount,
				FollowerCount: fn.FollowerCount,
				EventCount:    fn.EventCount,
				IsUnclaimed:   fn.Status == "unclaimed",
				Children:      []TreeNode{},
			})
		}

		root := TreeNode{
			ID:       "root",
			Name:     "Root",
			Tags:     []string{},
			Children: children,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tree":     root,
			"affinity": links,
		})
	}
}

func addAffinity(m map[string]map[string]float64, a, b string, score float64) {
	if m[a] == nil {
		m[a] = make(map[string]float64)
	}
	if m[b] == nil {
		m[b] = make(map[string]float64)
	}
	m[a][b] += score
	m[b][a] += score
}
