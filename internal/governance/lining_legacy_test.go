package governance

import "testing"

// The legacy bodies must stay byte-identical to what actually shipped as
// DefaultLiningBody — live instances carry these exact bytes in their
// governance_docs rows, and the startup heal matches by hash. An edit here
// (even whitespace) silently turns every healable lining into "diverged".
// If this test fails, revert the edit; these constants are frozen.
func TestLegacyLiningBodiesAreFrozen(t *testing.T) {
	frozen := map[string]string{
		"legacyLiningOriginal":  "d9b51d1957e2bdbde481a43cbcafa1adb08bcc1fc1327c5004b2691ab24a212f",
		"legacyLiningHumanized": "f03ebd5866a55e8e7a096b3898609a3dbdd54ab78dbbb0584a8f4c637a0a24c4",
	}
	got := map[string]string{
		"legacyLiningOriginal":  liningHash(legacyLiningOriginal),
		"legacyLiningHumanized": liningHash(legacyLiningHumanized),
	}
	for name, want := range frozen {
		if got[name] != want {
			t.Errorf("%s hash = %s, want %s — this constant is frozen; revert the edit", name, got[name], want)
		}
	}
}

// Every legacy body must classify as stale — that is the entire point of the
// list: pre-lineage defaults heal to the current lining instead of wearing
// the diverged badge.
func TestLegacyLiningBodiesHeal(t *testing.T) {
	for i, body := range legacyLiningBodies {
		if got := LiningStatus(body); got != LiningStale {
			t.Errorf("legacyLiningBodies[%d]: LiningStatus = %q, want %q", i, got, LiningStale)
		}
	}
}
