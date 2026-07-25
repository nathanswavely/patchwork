package weblink

import "testing"

// Every path this package builds, spelled out. These are assertions about
// web/src/App.svelte's route table, so a failure here means one of two
// things: the SPA route moved and this package needs to follow it, or a
// helper drifted. The mirror of this list on the frontend side lives in
// web/src/test/notification-links.test.js, which resolves each shape
// against the real router.
func TestPaths(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"patch", Patch("gallery-row"), "/patches/gallery-row"},
		{"patch events", PatchEvents("gallery-row"), "/patches/gallery-row/events"},
		{"patch members", PatchMembers("gallery-row"), "/patches/gallery-row/members"},
		{"pending members", PatchMembersPending("gallery-row"), "/patches/gallery-row/members?status=pending"},
		{"patch setup", PatchSetup("gallery-row"), "/patches/gallery-row/setup"},
		{"event", Event("ev-1"), "/events/ev-1"},
		{"proposal", Proposal("gallery-row", "pr-1"), "/patches/gallery-row/governance/pr-1"},
		{"governance doc", GovernanceDoc("gallery-row", "doc-1"), "/patches/gallery-row/governance/docs/doc-1"},
		{"remote patch", RemotePatch("other.example", "their-patch"), "/quilts/other.example/patches/their-patch"},
		{"absolute", Absolute("arts.example", "/events/ev-1"), "https://arts.example/events/ev-1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}

// The regression that produced this package: an event's path is global, and
// a charter's path carries `docs/`. Asserted on their own so the reason
// survives even if the table above is edited.
func TestEventPathIsNotPatchScoped(t *testing.T) {
	if got := Event("ev-1"); got != "/events/ev-1" {
		t.Errorf("event paths are global, not scoped under a patch: got %q", got)
	}
}

func TestGovernanceDocIsNotAProposalPath(t *testing.T) {
	doc := GovernanceDoc("gallery-row", "x")
	if doc == Proposal("gallery-row", "x") {
		t.Errorf("charter and proposal paths must differ; both were %q", doc)
	}
}
