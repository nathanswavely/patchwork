package handler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"log"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// pkgNotifier is the package-level notifier, set via SetNotifier from main.go.
// This avoids changing every handler function signature.
var pkgNotifier *notifications.Notifier

// SetNotifier sets the package-level notifier used by handlers.
func SetNotifier(n *notifications.Notifier) {
	pkgNotifier = n
}

// notify is a shorthand that safely calls Notify if the notifier is set.
func notify(event notifications.Event) {
	if pkgNotifier != nil {
		go pkgNotifier.Notify(event)
	}
}

// scanNodeLinks scans a JSON string into []NodeLink and assigns to node.
func scanNodeLinks(linksJSON string, n *model.Node) {
	if linksJSON != "" && linksJSON != "[]" {
		json.Unmarshal([]byte(linksJSON), &n.Links)
	}
	if n.Links == nil {
		n.Links = []model.NodeLink{}
	}
}

// scanFollowerPermissions scans a JSON string into FollowerPermissions and assigns to node.
func scanFollowerPermissions(fpJSON string, n *model.Node) {
	fp := &model.FollowerPermissions{Events: true, Proposals: true, Charters: true, Members: true}
	if fpJSON != "" && fpJSON != "{}" {
		json.Unmarshal([]byte(fpJSON), fp)
	}
	n.FollowerPermissions = fp
}

// scanAppearance scans a JSON string into Appearance and assigns to node.
// Empty/blank stays nil — unset appearance is a meaningful state (the tile
// is hash-assigned from the patch ID).
func scanAppearance(apJSON string, n *model.Node) {
	if apJSON == "" || apJSON == "{}" {
		return
	}
	a := &model.Appearance{}
	if err := json.Unmarshal([]byte(apJSON), a); err == nil {
		n.Appearance = a
	}
}

var appearanceSlugRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// Drafted-block limits (docs/adr/029). Coordinates are integers in
// quarter-cell units: a grid-n block spans 0..4n, so cell walls sit on
// multiples of 4 and the 25/50/75% anchor subdivisions land on integers.
const (
	draftMaxGrid           = 10
	draftSeamBudget        = 24
	draftBundleSlots       = 6
	draftFineAnchorMaxGrid = 5 // above this, anchors are midpoints only
	appearanceMaxBytes     = 8192
)

var draftCellKeyRe = regexp.MustCompile(`^(\d{1,2}),(\d{1,2})$`)

// draftBlock is the canonical shape of an embedded drafted block. It is
// validated structurally — grid bounds, seam budget, anchor legality, slot
// ranges — never aesthetically (docs/adr/029).
type draftBlock struct {
	Grid   int              `json:"grid"`
	Seams  [][4]int         `json:"seams,omitempty"`
	Colors map[string][]int `json:"colors,omitempty"`
}

// legalAnchor reports whether (x, y) is a seam anchor on a grid-n block:
// on a cell wall, within bounds, at a quarter subdivision (halves only
// above 5x5 — finer grids get expressiveness from density instead).
func legalAnchor(grid, x, y int) bool {
	max := 4 * grid
	if x < 0 || y < 0 || x > max || y > max {
		return false
	}
	if x%4 != 0 && y%4 != 0 {
		return false
	}
	if grid > draftFineAnchorMaxGrid && (x%2 != 0 || y%2 != 0) {
		return false
	}
	return true
}

// normalizeDraftBlock validates a decoded drafted-block object and returns
// its canonical form.
func normalizeDraftBlock(m map[string]interface{}) (*draftBlock, error) {
	for k := range m {
		switch k {
		case "grid", "seams", "colors":
		default:
			return nil, fmt.Errorf("unknown appearance.block field %q", k)
		}
	}
	gf, ok := m["grid"].(float64)
	g := int(gf)
	if !ok || float64(g) != gf || g < 1 || g > draftMaxGrid {
		return nil, fmt.Errorf("appearance.block.grid must be an integer 1-%d", draftMaxGrid)
	}
	d := &draftBlock{Grid: g}

	if sv, present := m["seams"]; present {
		arr, ok := sv.([]interface{})
		if !ok {
			return nil, fmt.Errorf("appearance.block.seams must be an array")
		}
		if len(arr) > draftSeamBudget {
			return nil, fmt.Errorf("appearance.block.seams exceeds the seam budget of %d", draftSeamBudget)
		}
		for _, e := range arr {
			quad, ok := e.([]interface{})
			if !ok || len(quad) != 4 {
				return nil, fmt.Errorf("each seam must be [x1, y1, x2, y2]")
			}
			var pts [4]int
			for i, cv := range quad {
				f, ok := cv.(float64)
				n := int(f)
				if !ok || float64(n) != f {
					return nil, fmt.Errorf("seam coordinates must be integers in quarter-cell units")
				}
				pts[i] = n
			}
			if !legalAnchor(g, pts[0], pts[1]) || !legalAnchor(g, pts[2], pts[3]) {
				return nil, fmt.Errorf("seam endpoints must be anchors on cell walls (quarter points up to 5x5, midpoints above)")
			}
			if pts[0] == pts[2] && pts[1] == pts[3] {
				return nil, fmt.Errorf("a seam must connect two distinct anchors")
			}
			d.Seams = append(d.Seams, pts)
		}
	}

	if cv, present := m["colors"]; present {
		cm, ok := cv.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf(`appearance.block.colors must be an object keyed by "row,col"`)
		}
		d.Colors = make(map[string][]int, len(cm))
		for key, sv := range cm {
			mm := draftCellKeyRe.FindStringSubmatch(key)
			if mm == nil {
				return nil, fmt.Errorf(`appearance.block.colors keys must be "row,col"`)
			}
			r, _ := strconv.Atoi(mm[1])
			c, _ := strconv.Atoi(mm[2])
			if r >= g || c >= g {
				return nil, fmt.Errorf("appearance.block.colors cell %q is outside the %dx%d grid", key, g, g)
			}
			slots, ok := sv.([]interface{})
			if !ok {
				return nil, fmt.Errorf("appearance.block.colors values must be arrays of bundle slots")
			}
			out := make([]int, 0, len(slots))
			for _, e := range slots {
				f, ok := e.(float64)
				n := int(f)
				if !ok || float64(n) != f || n < 0 || n >= draftBundleSlots {
					return nil, fmt.Errorf("bundle slots must be integers 0-%d", draftBundleSlots-1)
				}
				out = append(out, n)
			}
			d.Colors[key] = out
		}
	}
	return d, nil
}

// normalizeAppearance validates a decoded appearance request value and
// returns the canonical JSON string to store, or nil for SQL NULL (unset).
// Slug values (palette, icon, curated block) are validated for shape only —
// membership is the frontend registry's concern, and unknown keys fall back
// to hash assignment there (docs/adr/004). A drafted block object is
// validated structurally per docs/adr/029.
func normalizeAppearance(v interface{}) (*string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("appearance must be an object or null")
	}
	a := model.Appearance{}
	for k, val := range m {
		switch k {
		case "palette", "icon":
			s, ok := val.(string)
			if !ok || !appearanceSlugRe.MatchString(s) {
				return nil, fmt.Errorf("appearance.%s must be a slug of 1-32 chars [a-zA-Z0-9_-]", k)
			}
			if k == "palette" {
				a.Palette = s
			} else {
				a.Icon = s
			}
		case "block":
			switch bv := val.(type) {
			case string:
				if !appearanceSlugRe.MatchString(bv) {
					return nil, fmt.Errorf("appearance.block must be a slug of 1-32 chars [a-zA-Z0-9_-]")
				}
				a.Block, _ = json.Marshal(bv)
			case map[string]interface{}:
				d, err := normalizeDraftBlock(bv)
				if err != nil {
					return nil, err
				}
				b, err := json.Marshal(d)
				if err != nil {
					return nil, err
				}
				a.Block = b
			default:
				return nil, fmt.Errorf("appearance.block must be a slug or a drafted block object")
			}
		case "bundle":
			arr, ok := val.([]interface{})
			if !ok || len(arr) == 0 || len(arr) > draftBundleSlots {
				return nil, fmt.Errorf("appearance.bundle must be an array of 1-%d hex colors", draftBundleSlots)
			}
			for _, e := range arr {
				s, ok := e.(string)
				if !ok || !hexColorRE.MatchString(s) {
					return nil, fmt.Errorf("appearance.bundle entries must be hex colors like #RRGGBB")
				}
				a.Bundle = append(a.Bundle, s)
			}
		case "rotation":
			f, ok := val.(float64)
			r := int(f)
			if !ok || float64(r) != f || (r != 0 && r != 90 && r != 180 && r != 270) {
				return nil, fmt.Errorf("appearance.rotation must be one of 0, 90, 180, 270")
			}
			a.Rotation = &r
		default:
			return nil, fmt.Errorf("unknown appearance field %q", k)
		}
	}
	if a.Palette == "" && len(a.Block) == 0 && a.Rotation == nil && len(a.Bundle) == 0 && a.Icon == "" {
		return nil, nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	if len(b) > appearanceMaxBytes {
		return nil, fmt.Errorf("appearance exceeds %d bytes", appearanceMaxBytes)
	}
	s := string(b)
	return &s, nil
}

// scanGovernanceConfig scans a JSON string into GovernanceConfig and assigns to node.
func scanGovernanceConfig(gcJSON string, n *model.Node) {
	gc := &model.GovernanceConfig{
		DecisionMethod:      "majority",
		QuorumPercent:       0,
		DefaultVoteDuration: 72,
		AmendmentThreshold:  "majority",
		AmendmentAutoApply:  true,
		SuccessionPolicy:    "longest_tenure",
		MinVotingTenureDays: 0,
	}
	if gcJSON != "" && gcJSON != "{}" {
		json.Unmarshal([]byte(gcJSON), gc)
	}
	n.GovernanceConfig = gc
}

// validateCoordinate checks a latitude or longitude value taken from a JSON
// request. A nil value is an explicit null — clearing the position — and is
// always allowed; a present value must be a number within its geographic
// range (lat −90..90, lng −180..180). Placement is map-drag only, so the
// numbers a client can send are already bounded, but a stored out-of-range
// coordinate would put a marker nowhere or throw off the map's fit, so it is
// rejected rather than clamped.
func validateCoordinate(field string, val interface{}) (interface{}, error) {
	if val == nil {
		return nil, nil
	}
	f, ok := val.(float64)
	if !ok {
		return nil, fmt.Errorf("%s must be a number or null", field)
	}
	switch field {
	case "latitude":
		if f < -90 || f > 90 {
			return nil, fmt.Errorf("latitude must be between -90 and 90")
		}
	case "longitude":
		if f < -180 || f > 180 {
			return nil, fmt.Errorf("longitude must be between -180 and 180")
		}
	}
	return f, nil
}

func generateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "node"
	}
	return s
}

func uniqueSlug(db *database.DB, base string) string {
	slug := base
	for {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM nodes WHERE slug = ?", slug).Scan(&count)
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%04x", base, rand.Intn(0xFFFF))
	}
}

func parsePaginationParams(r *http.Request) (string, int) {
	after := r.URL.Query().Get("after")
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		if l > 100 {
			l = 100
		}
		limit = l
	}
	return after, limit
}

// NodeIDFromSlug resolves a node slug to its ID. Returns empty string if not found.
func NodeIDFromSlug(db *database.DB, slug string) string {
	var id string
	db.QueryRow("SELECT id FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", slug).Scan(&id)
	return id
}

// userHasMembership checks if a user has an active membership in a node.
func userHasMembership(db *database.DB, userID, nodeID string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'", userID, nodeID).Scan(&count)
	return count > 0
}

// userHasNodeRole checks if a user has a specific role (or higher) on a node.
func userHasNodeRole(db *database.DB, userID, nodeID string, roles ...string) bool {
	placeholders := make([]string, len(roles))
	args := make([]interface{}, 0, len(roles)+2)
	args = append(args, userID, nodeID)
	for i, r := range roles {
		placeholders[i] = "?"
		args = append(args, r)
	}
	var count int
	db.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN (%s)", strings.Join(placeholders, ",")),
		args...,
	).Scan(&count)
	return count > 0
}

// ListNodes handles GET /api/v1/nodes.
func ListNodes(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)
		tag := r.URL.Query().Get("tag")
		search := r.URL.Query().Get("search")
		visibility := r.URL.Query().Get("visibility")
		scope := r.URL.Query().Get("scope")
		user := middleware.UserFromContext(r.Context())

		// "My Quilt" scope. Mirrors GET /api/v1/nodes/tree so the map and the
		// quilt show the same set of patches (issue #45).
		myScope := scope == "my"
		if myScope && user == nil {
			// A personal quilt has no meaning without a viewer. The tree
			// handler silently falls through to the whole instance here;
			// that is not a behaviour worth copying — answering "show me
			// mine" with "here is everyone's" is worse than answering with
			// nothing. Anonymous callers get an empty page, not the instance.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"items":       []model.Node{},
				"next_cursor": "",
			})
			return
		}

		query := "SELECT n.id, n.owner_id, n.name, n.slug, n.description, n.latitude, n.longitude, n.address, n.website, COALESCE(n.links,'[]'), COALESCE(n.follower_permissions,'{}'), COALESCE(n.governance_config,'{}'), n.visibility, n.membership_policy, COALESCE(n.appearance,''), n.created_at, n.updated_at FROM nodes n"
		var conditions []string
		var args []interface{}

		conditions = append(conditions, "n.status IN ('active','unclaimed')")
		conditions = append(conditions, "n.removed_at IS NULL")

		// This JOIN carries a placeholder, and it sits textually before every
		// WHERE condition — so its argument must be bound first. Keep this
		// block above the tag block.
		if myScope {
			// Every active relationship counts: admin, member, AND follower.
			// A follower is an interested observer, not a member, but the
			// patch is still theirs on their quilt — so no role filter here,
			// matching the tree handler's join.
			query += " JOIN memberships mem ON mem.node_id = n.id AND mem.user_id = ? AND mem.status = 'active'"
			args = append(args, user.ID)
		}

		if tag != "" {
			query += " JOIN node_tags nt ON n.id = nt.node_id JOIN tags t ON nt.tag_id = t.id"
			conditions = append(conditions, "t.name = ?")
			args = append(args, tag)
		}

		// Visibility. The default listing is public-only; My Quilt deliberately
		// is not. Every row the scoped query can reach is one the caller holds
		// an active membership on, so a private patch they belong to is already
		// theirs to see — the tree handler shows it on the quilt, and the map
		// has to agree or markers disappear with no reason the viewer can read.
		// The scoping is by the caller's own membership, so this widens nothing
		// for anyone else. An explicit ?visibility= still narrows, as always.
		if !myScope && visibility == "" {
			visibility = "public"
		}
		if visibility != "" {
			conditions = append(conditions, "n.visibility = ?")
			args = append(args, visibility)
		}

		if search != "" {
			conditions = append(conditions, "(n.name LIKE ? OR n.description LIKE ?)")
			s := "%" + search + "%"
			args = append(args, s, s)
		}

		if r.URL.Query().Get("has_location") == "true" {
			conditions = append(conditions, "n.latitude IS NOT NULL AND n.longitude IS NOT NULL")
		}

		// Amended-lining discovery filter (docs/adr/036). Excluded in SQL so
		// cursor pagination stays exact. Never applies to My Quilt — a patch
		// you belong to is yours to see.
		if !myScope && hideAmendedLinings(db, r) {
			for nodeID, status := range NodeLiningStatuses(db) {
				if status == governance.LiningDiverged {
					conditions = append(conditions, "n.id != ?")
					args = append(args, nodeID)
				}
			}
		}

		if after != "" {
			conditions = append(conditions, "n.id > ?")
			args = append(args, after)
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}

		query += " ORDER BY n.id ASC LIMIT ?"
		args = append(args, limit+1) // fetch one extra to determine next_cursor

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list nodes"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var nodes []model.Node
		for rows.Next() {
			var n model.Node
			var linksJSON, fpJSON, gcJSON, apJSON string
			if err := rows.Scan(&n.ID, &n.OwnerID, &n.Name, &n.Slug, &n.Description, &n.Latitude, &n.Longitude, &n.Address, &n.Website, &linksJSON, &fpJSON, &gcJSON, &n.Visibility, &n.MembershipPolicy, &apJSON, &n.CreatedAt, &n.UpdatedAt); err != nil {
				continue
			}
			scanNodeLinks(linksJSON, &n)
			scanFollowerPermissions(fpJSON, &n)
			scanGovernanceConfig(gcJSON, &n)
			scanAppearance(apJSON, &n)
			nodes = append(nodes, n)
		}

		var nextCursor string
		if len(nodes) > limit {
			nextCursor = nodes[limit-1].ID
			nodes = nodes[:limit]
		}

		if nodes == nil {
			nodes = []model.Node{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       nodes,
			"next_cursor": nextCursor,
		})
	}
}

// GetNode handles GET /api/v1/nodes/{slug}.
func GetNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if slug == "" {
			http.Error(w, `{"error":"slug required"}`, http.StatusBadRequest)
			return
		}

		var n model.Node
		var linksJSON, fpJSON, gcJSON, apJSON string
		err := db.QueryRow(
			`SELECT id, owner_id, name, slug, description, latitude, longitude, address, website, COALESCE(links,'[]'), COALESCE(follower_permissions,'{}'), COALESCE(governance_config,'{}'), visibility, membership_policy, COALESCE(appearance,''), status, COALESCE(submission_source,'owner'), accept_event_suggestions, created_at, updated_at
			 FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`, slug,
		).Scan(&n.ID, &n.OwnerID, &n.Name, &n.Slug, &n.Description, &n.Latitude, &n.Longitude, &n.Address, &n.Website, &linksJSON, &fpJSON, &gcJSON, &n.Visibility, &n.MembershipPolicy, &apJSON, &n.Status, &n.SubmissionSource, &n.AcceptEventSuggestions, &n.CreatedAt, &n.UpdatedAt)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		scanNodeLinks(linksJSON, &n)
		scanFollowerPermissions(fpJSON, &n)
		scanGovernanceConfig(gcJSON, &n)
		scanAppearance(apJSON, &n)

		// Tags in stored (priority) order, matching the tree endpoint — the
		// appearance settings page needs them to show the effective
		// (tag-derived) motif, and order decides which tag derives it.
		n.Tags = nodeTagNames(db, n.ID)

		// Same counts the tree endpoint reports, so cards and profile agree:
		// members are admins + members; followers are counted separately and
		// never conflated (a follower is an observer, not a member).
		db.QueryRow(
			`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND status = 'active' AND role IN ('admin','member')`, n.ID,
		).Scan(&n.MemberCount)
		db.QueryRow(
			`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND status = 'active' AND role = 'follower'`, n.ID,
		).Scan(&n.FollowerCount)

		isUnclaimed := n.Status == "unclaimed"

		// Include owner info (hide for unclaimed patches — owner is system user).
		resp := map[string]interface{}{
			"node":         n,
			"is_unclaimed": isUnclaimed,
		}
		// Lining status is deliberately public (docs/adr/036): "amended
		// lining" (diverged) is the badge state the whole design hangs on.
		var liningBody string
		if db.QueryRow("SELECT body FROM governance_docs WHERE node_id = ? AND kind = 'lining'", n.ID).Scan(&liningBody) == nil {
			resp["lining_status"] = governance.LiningStatus(liningBody)
		}
		// The verification domain is the trust anchor for claims (docs/adr/030);
		// surface it for unclaimed patches so the admin's Verification settings
		// can show and edit it. It is already public via the claim endpoints.
		if isUnclaimed {
			var vd string
			db.QueryRow("SELECT COALESCE(verification_domain,'') FROM nodes WHERE id = ?", n.ID).Scan(&vd)
			resp["verification_domain"] = vd
		}
		if !isUnclaimed {
			var owner model.User
			db.QueryRow(
				`SELECT id, username, display_name, avatar_url FROM users WHERE id = ?`, n.OwnerID,
			).Scan(&owner.ID, &owner.Username, &owner.DisplayName, &owner.AvatarURL)
			resp["owner"] = owner
		}
		if user := middleware.UserFromContext(r.Context()); user != nil {
			var role, memStatus string
			err := db.QueryRow(
				"SELECT role, status FROM memberships WHERE user_id = ? AND node_id = ?",
				user.ID, n.ID,
			).Scan(&role, &memStatus)
			if err == nil {
				if memStatus == "active" {
					resp["is_member"] = true
					resp["is_admin"] = role == "admin"
					resp["membership_role"] = role
				} else if memStatus == "banned" {
					resp["is_banned"] = true
				}
			}
			// Global admins can manage any patch (matches the write-side
			// authz: every admin-gated node endpoint accepts user.Role ==
			// "admin"). Surface that so the UI shows the management surface.
			if user.Role == "admin" {
				resp["is_admin"] = true
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// CreateNode handles POST /api/v1/nodes.
func CreateNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name                string                     `json:"name"`
			Description         string                     `json:"description"`
			Latitude            *float64                   `json:"latitude"`
			Longitude           *float64                   `json:"longitude"`
			Address             string                     `json:"address"`
			Website             string                     `json:"website"`
			Links               []model.NodeLink           `json:"links"`
			Visibility          string                     `json:"visibility"`
			MembershipPolicy    string                     `json:"membership_policy"`
			Appearance          interface{}                `json:"appearance"`
			Template            string                     `json:"template"`
			FollowerPermissions *model.FollowerPermissions `json:"follower_permissions"`
			Tags                []string                   `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}
		if req.Visibility == "" {
			req.Visibility = "public"
		}
		if req.MembershipPolicy == "" {
			req.MembershipPolicy = "open"
		}

		if req.Latitude != nil {
			if _, err := validateCoordinate("latitude", *req.Latitude); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
		}
		if req.Longitude != nil {
			if _, err := validateCoordinate("longitude", *req.Longitude); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
				return
			}
		}

		id := auth.NewUUIDv7()
		slug := uniqueSlug(db, generateSlug(req.Name))

		linksStr := "[]"
		if len(req.Links) > 0 {
			lb, _ := json.Marshal(req.Links)
			linksStr = string(lb)
		}

		fpStr := "{}"
		if req.FollowerPermissions != nil {
			fb, _ := json.Marshal(req.FollowerPermissions)
			fpStr = string(fb)
		}

		appearanceStr, apErr := normalizeAppearance(req.Appearance)
		if apErr != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, apErr.Error()), http.StatusBadRequest)
			return
		}

		// Tags come from the curated vocabulary only; validate before insert.
		tagIDs, unknownTag := resolveTagIDs(db, req.Tags)
		if unknownTag != "" {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "unknown tag: "+unknownTag), http.StatusBadRequest)
			return
		}

		apID := ap.NodeAPID(ap.GetDomain(), id)
		_, err := db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, follower_permissions, visibility, membership_policy, appearance, ap_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, user.ID, req.Name, slug, req.Description, req.Latitude, req.Longitude, req.Address, req.Website, linksStr, fpStr, req.Visibility, req.MembershipPolicy, appearanceStr, apID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create node"}`, http.StatusInternalServerError)
			return
		}

		// Generate keypair for ActivityPub federation.
		ap.EnsureNodeKeypair(db, id)

		// Array order is the stored order — the admin's priority order
		// (first motif-bearing tag derives the motif; docs/adr/021).
		if len(tagIDs) > 0 {
			setNodeTags(db, id, tagIDs)
		}

		// Auto-create admin membership for the creator.
		memID := auth.NewUUIDv7()
		db.Exec(
			`INSERT INTO memberships (id, user_id, node_id, role, status) VALUES (?, ?, ?, 'admin', 'active')`,
			memID, user.ID, id,
		)

		// Auto-create default governance doc (lining).
		CreateDefaultLining(db, id, user.ID)

		// Fork governance repo for the new patch.
		if err := governance.ForkForNode(governance.GetDataDir(), id, req.Template); err != nil {
			log.Printf("warning: governance fork for node %s: %v", id, err)
		}

		auth.LogAuditEvent(db, user.ID, "node.create", "node", id, "{}", clientIP(r))
		auth.LogAuditEvent(db, user.ID, "membership.join", "membership", memID, `{"role":"admin","auto":true}`, clientIP(r))

		var n model.Node
		var linksJSON, fpJSON, gcJSON, apJSON string
		db.QueryRow(
			`SELECT id, owner_id, name, slug, description, latitude, longitude, address, website, COALESCE(links,'[]'), COALESCE(follower_permissions,'{}'), COALESCE(governance_config,'{}'), visibility, membership_policy, COALESCE(appearance,''), created_at, updated_at
			 FROM nodes WHERE id = ?`, id,
		).Scan(&n.ID, &n.OwnerID, &n.Name, &n.Slug, &n.Description, &n.Latitude, &n.Longitude, &n.Address, &n.Website, &linksJSON, &fpJSON, &gcJSON, &n.Visibility, &n.MembershipPolicy, &apJSON, &n.CreatedAt, &n.UpdatedAt)
		scanNodeLinks(linksJSON, &n)
		scanFollowerPermissions(fpJSON, &n)
		scanGovernanceConfig(gcJSON, &n)
		scanAppearance(apJSON, &n)
		n.Tags = nodeTagNames(db, id)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(n)
	}
}

// UpdateNode handles PATCH /api/v1/nodes/{slug}.
func UpdateNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Check permissions: admin on node or global admin.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		allowedFields := map[string]bool{
			"name": true, "description": true,
			"latitude": true, "longitude": true, "address": true,
			"website": true, "links": true, "visibility": true,
			"appearance": true, "accept_event_suggestions": true,
		}

		var setClauses []string
		var args []interface{}
		tagsUpdated := false
		for field, val := range req {
			// tags live in node_tags, not a nodes column. Validate against
			// the vocabulary and replace the set; array order is the stored
			// (priority) order. Instance admins pass the permission check
			// above by design — they may seed any patch's tags, and a patch
			// admin's later edit simply wins (docs/adr/021).
			if field == "tags" {
				rawList, ok := val.([]interface{})
				if !ok {
					http.Error(w, `{"error":"tags must be an array of tag names"}`, http.StatusBadRequest)
					return
				}
				names := make([]string, 0, len(rawList))
				for _, rv := range rawList {
					name, ok := rv.(string)
					if !ok {
						http.Error(w, `{"error":"tags must be an array of tag names"}`, http.StatusBadRequest)
						return
					}
					names = append(names, name)
				}
				tagIDs, unknownTag := resolveTagIDs(db, names)
				if unknownTag != "" {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, "unknown tag: "+unknownTag), http.StatusBadRequest)
					return
				}
				if err := setNodeTags(db, nodeID, tagIDs); err != nil {
					http.Error(w, `{"error":"failed to update tags"}`, http.StatusInternalServerError)
					return
				}
				tagsUpdated = true
				continue
			}
			if !allowedFields[field] {
				continue
			}
			// links is stored as a JSON string in the DB
			if field == "links" {
				linksBytes, err := json.Marshal(val)
				if err != nil {
					continue
				}
				setClauses = append(setClauses, field+" = ?")
				args = append(args, string(linksBytes))
				continue
			}
			// latitude/longitude drive the map marker. A number in range sets
			// the position; an explicit null clears it (unset position = off
			// the map — there is no show_on_map flag). Out-of-range or
			// non-numeric values are rejected, not stored.
			if field == "latitude" || field == "longitude" {
				coord, err := validateCoordinate(field, val)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
					return
				}
				setClauses = append(setClauses, field+" = ?")
				args = append(args, coord)
				continue
			}
			// appearance is shape-validated and stored as canonical JSON
			// (or NULL when cleared back to unset).
			if field == "appearance" {
				apStr, err := normalizeAppearance(val)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
					return
				}
				setClauses = append(setClauses, field+" = ?")
				args = append(args, apStr)
				continue
			}
			setClauses = append(setClauses, field+" = ?")
			args = append(args, val)
		}

		if len(setClauses) == 0 && !tagsUpdated {
			http.Error(w, `{"error":"no valid fields to update"}`, http.StatusBadRequest)
			return
		}

		setClauses = append(setClauses, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
		args = append(args, nodeID)

		_, err := db.Exec(
			fmt.Sprintf("UPDATE nodes SET %s WHERE id = ?", strings.Join(setClauses, ", ")),
			args...,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update node"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.update", "node", nodeID, "{}", clientIP(r))

		var n model.Node
		var linksJSON, fpJSON, gcJSON, apJSON string
		db.QueryRow(
			`SELECT id, owner_id, name, slug, description, latitude, longitude, address, website, COALESCE(links,'[]'), COALESCE(follower_permissions,'{}'), COALESCE(governance_config,'{}'), visibility, membership_policy, COALESCE(appearance,''), created_at, updated_at
			 FROM nodes WHERE id = ?`, nodeID,
		).Scan(&n.ID, &n.OwnerID, &n.Name, &n.Slug, &n.Description, &n.Latitude, &n.Longitude, &n.Address, &n.Website, &linksJSON, &fpJSON, &gcJSON, &n.Visibility, &n.MembershipPolicy, &apJSON, &n.CreatedAt, &n.UpdatedAt)
		scanNodeLinks(linksJSON, &n)
		scanFollowerPermissions(fpJSON, &n)
		scanGovernanceConfig(gcJSON, &n)
		scanAppearance(apJSON, &n)
		n.Tags = nodeTagNames(db, nodeID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(n)
	}
}

// DeleteNode handles DELETE /api/v1/nodes/{slug}.
func DeleteNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Require admin role on node or global admin.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		// archived_from remembers what restore returns the patch to
		// (docs/adr/034); the RHS status reads the pre-update row.
		_, err := db.Exec("UPDATE nodes SET archived_from = status, status = 'archived', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?", nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to archive node"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.delete", "node", nodeID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
