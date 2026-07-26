package notifications

import "testing"

// Every registered type must be reachable through its category.
//
// TypesForCategory walks a hand-written slice rather than the registry, so a
// type can be fully wired — firing, delivering, landing in someone's inbox —
// and still be missing from the list. The preferences UI is built from these
// categories, so the only symptom is that nobody can turn the notification
// off. A type that mails people and cannot be muted is the worst version of
// this, and that is exactly what the mid-vote rules notice is.
func TestEveryRegisteredTypeIsEnumerated(t *testing.T) {
	enumerated := make(map[NotificationType]bool)
	for _, c := range AllCategories() {
		for _, typ := range TypesForCategory(c.ID) {
			enumerated[typ] = true
		}
	}
	for typ := range TypeRegistry {
		if !enumerated[typ] {
			t.Errorf("%q is registered but not in TypesForCategory's list, so it cannot be muted", typ)
		}
	}
}

// The two rules-change notices are split so they can be muted and mailed
// separately (docs/adr/047). If they carried the same priority the split would
// buy only the muting half, and the routine one would go back to mailing every
// member of every patch on every config edit.
func TestRulesChangeNoticesDifferInUrgency(t *testing.T) {
	routine, ok := TypeRegistry[GovernanceRulesChanged]
	if !ok {
		t.Fatal("GovernanceRulesChanged is not registered")
	}
	midVote, ok := TypeRegistry[GovernanceRulesChangedMidVote]
	if !ok {
		t.Fatal("GovernanceRulesChangedMidVote is not registered")
	}

	if DefaultEnabled(GovernanceRulesChanged, "email") {
		t.Error("a routine rules edit should not email by default")
	}
	if !DefaultEnabled(GovernanceRulesChangedMidVote, "email") {
		t.Error("a rules edit that misses running votes should email by default")
	}
	if routine.Audience != midVote.Audience {
		t.Errorf("audiences differ (%v vs %v) — both go to the patch's members",
			routine.Audience, midVote.Audience)
	}
	if routine.Category != CategoryGovernance || midVote.Category != CategoryGovernance {
		t.Error("both belong to the governance category")
	}
}
