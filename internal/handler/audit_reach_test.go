package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The audit mark for the instance-admin reach
// (docs/adr/2026-10-20-an-act-the-instance-role-let-through-says-so.md).
//
// One edit, three callers. The patch's own admin and an instance admin who is
// also the patch's admin act on standing, and their entries look exactly as
// they did. An instance admin holding no role acts on the instance role
// alone, and only that entry says so. cmd/patchwork's route walk checks the
// mark is on every entry such a caller writes; this checks it is on nothing
// else.
func TestAnActTheInstanceRoleLetThroughSaysSo(t *testing.T) {
	db := setupTestDB(t)

	owner, ownerToken := createTestUser(t, db, "weaver", "member")
	nodeID := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, nodeID, "admin", "active")

	both, bothToken := createTestUser(t, db, "both", "admin")
	createTestMembership(t, db, both.ID, nodeID, "admin", "active")

	member, memberToken := createTestUser(t, db, "member-steward", "admin")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	_, stewardToken := createTestUser(t, db, "steward", "admin")

	cases := []struct {
		name   string
		token  string
		marked bool
	}{
		{"the patch's own admin", ownerToken, false},
		{"an instance admin who is also the patch's admin", bothToken, false},
		{"an instance admin who is only a member", memberToken, true},
		{"an instance admin holding no role", stewardToken, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := authedRequest("PATCH", "/api/v1/nodes/gallery-row", map[string]any{"description": tc.name}, tc.token)
			w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
			if w.Code != 200 {
				t.Fatalf("PATCH answered %d: %s", w.Code, w.Body.String())
			}

			detail := lastAuditDetail(t, db, "node.update")
			if got := detail["acting_without_role"] == true; got != tc.marked {
				t.Errorf("acting_without_role = %v, want %v (detail %v)", detail["acting_without_role"], tc.marked, detail)
			}
			if _, present := detail["acting_without_role"]; present && !tc.marked {
				t.Errorf("an act on standing carries the key at all (detail %v); it should be absent, not false", detail)
			}
			// The mark is added to the entry, never in place of it.
			if fields, _ := detail["fields"].([]any); len(fields) != 1 || fields[0] != "description" {
				t.Errorf("fields = %v, want [description]", detail["fields"])
			}
		})
	}
}

func lastAuditDetail(t *testing.T, db *database.DB, action string) map[string]any {
	t.Helper()
	var metadata string
	if err := db.QueryRow(
		`SELECT metadata FROM audit_log WHERE action = ? ORDER BY rowid DESC LIMIT 1`, action,
	).Scan(&metadata); err != nil {
		t.Fatalf("no %s entry: %v", action, err)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(metadata), &detail); err != nil {
		t.Fatalf("%s metadata is not a JSON object: %q", action, metadata)
	}
	return detail
}
