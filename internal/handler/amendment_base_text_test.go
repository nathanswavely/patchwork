package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// An applied amendment's page shows what it changed. Once it is in effect
// the current text *is* the proposed text, so a client diffing proposed
// against `current_doc_content` showed an empty change on the change's own
// page. `base_doc_content` is the text it replaced.

const amendDoc = "operating-agreement.md"

func docText(t *testing.T, nodeID string) string {
	t.Helper()
	text, err := governance.GetDocument(governance.GetDataDir(), nodeID, amendDoc)
	if err != nil {
		t.Fatalf("read %s: %v", amendDoc, err)
	}
	return text
}

func amend(t *testing.T, rig adminDecidesRig, token, proposed string, putToVote bool) string {
	t.Helper()
	code, p := createProposal(t, rig.db, rig.slug, token, map[string]interface{}{
		"title": "Amend the agreement", "proposal_type": "amendment",
		"target_doc": amendDoc, "proposed_body": proposed, "put_to_vote": putToVote,
	})
	if code != http.StatusCreated {
		t.Fatalf("create amendment: got %d: %v", code, p)
	}
	return p["id"].(string)
}

// The ordinary case: nothing moved between draft and apply, so the replaced
// text is the text it was written against.
func TestAppliedAmendment_CarriesTheTextItReplaced(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "basetext")
	original := docText(t, rig.nodeID)
	proposed := "# Operating Agreement\n\nRewritten.\n"

	id := amend(t, rig, rig.adminTok, proposed, false)
	got := getProposal(t, rig.db, id, rig.adminTok)

	if got["state"] != "in_effect" {
		t.Fatalf("expected the direct change in effect, got %v", got["state"])
	}
	if got["current_doc_content"] != proposed {
		t.Errorf("current_doc_content is still the current text, got %q", got["current_doc_content"])
	}
	if got["base_doc_content"] != original {
		t.Errorf("base_doc_content: want the text the amendment replaced, got %q", got["base_doc_content"])
	}
}

// While it is open there is nothing applied to be the base of, and the
// current text is the right thing to diff against.
func TestOpenAmendment_HasNoBaseText(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "basetextopen")
	id := amend(t, rig, rig.adminTok, "# Operating Agreement\n\nMaybe.\n", true)
	got := getProposal(t, rig.db, id, rig.adminTok)
	if _, ok := got["base_doc_content"]; ok {
		t.Errorf("an open amendment must not carry base_doc_content, got %q", got["base_doc_content"])
	}
}

// The document moved under an open amendment: another amendment landed first,
// so applying this one is a merge commit rather than a fast-forward. Its base
// is the text it displaced, which is the other amendment's.
func TestAppliedAmendment_AfterTheDocumentMoved(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "basetextmoved")
	original := docText(t, rig.nodeID)
	first := "# Operating Agreement\n\nThe first rewrite.\n"
	second := "# Operating Agreement\n\nThe second rewrite.\n"

	pending := amend(t, rig, rig.adminTok, second, true)
	landed := amend(t, rig, rig.adminTok, first, false)
	if code, body := decideOn(t, rig.db, pending, rig.adminTok, "approve"); code != http.StatusOK {
		t.Fatalf("decide approve: got %d: %s", code, body)
	}

	if got := getProposal(t, rig.db, landed, rig.adminTok)["base_doc_content"]; got != original {
		t.Errorf("the first amendment replaced the original, got %q", got)
	}
	got := getProposal(t, rig.db, pending, rig.adminTok)
	if got["base_doc_content"] != first {
		t.Errorf("the second amendment replaced the first's text, got %q", got["base_doc_content"])
	}
	if got["current_doc_content"] != second {
		t.Errorf("current_doc_content: want the second rewrite, got %q", got["current_doc_content"])
	}
}

// A revision rebuilds the branch from main as it then stands, which leaves
// base_sha behind. The base comes from the applied commit, so it follows.
func TestAppliedAmendment_AfterARevision(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "basetextrev")
	first := "# Operating Agreement\n\nLanded while the other was open.\n"
	revised := "# Operating Agreement\n\nRevised after that.\n"

	pending := amend(t, rig, rig.adminTok, "# Operating Agreement\n\nDraft.\n", true)
	amend(t, rig, rig.adminTok, first, false)

	r := authedRequest("POST", "/api/v1/proposals/"+pending+"/revisions",
		map[string]interface{}{"proposed_body": revised, "change_note": "Rebased on the new text"}, rig.adminTok)
	if w := serveMux(t, rig.db, "POST", "/api/v1/proposals/{id}/revisions", handler.CreateRevision(rig.db), r); w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("revise: got %d: %s", w.Code, w.Body.String())
	}
	if code, body := decideOn(t, rig.db, pending, rig.adminTok, "approve"); code != http.StatusOK {
		t.Fatalf("decide approve: got %d: %s", code, body)
	}

	if got := getProposal(t, rig.db, pending, rig.adminTok)["base_doc_content"]; got != first {
		t.Errorf("base_doc_content: want the text the revised amendment replaced, got %q", got)
	}
}

// An amendment that created its document replaced nothing.
func TestAppliedAmendment_CreatingADocumentHasAnEmptyBase(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "basetextnew")
	code, p := createProposal(t, rig.db, rig.slug, rig.adminTok, map[string]interface{}{
		"title": "A new charter", "proposal_type": "amendment",
		"target_doc": "house-rules.md", "proposed_body": "# House Rules\n\nBe kind.\n",
	})
	if code != http.StatusCreated {
		t.Fatalf("create amendment: got %d: %v", code, p)
	}
	got := getProposal(t, rig.db, p["id"].(string), rig.adminTok)
	if base, ok := got["base_doc_content"]; !ok || base != "" {
		t.Errorf("base_doc_content: want present and empty, got %q (present=%v)", base, ok)
	}
}

// The base text is the charter's text, so it is withheld exactly where the
// current and proposed text are (docs/adr/036).
func TestAppliedAmendment_BaseTextFollowsTheCharter(t *testing.T) {
	rig := newTemplateRig(t, "minimal", "basetexthidden")
	openGovernanceRecord(t, rig.db, rig.nodeID)
	if _, err := rig.db.Exec(`INSERT INTO governance_docs (id, node_id, title, body, visibility, created_by) VALUES (?, ?, 'Operating Agreement', '', 'members', ?)`,
		auth.NewUUIDv7(), rig.nodeID, rig.adminID); err != nil {
		t.Fatal(err)
	}
	id := amend(t, rig, rig.adminTok, "# Operating Agreement\n\nPrivate.\n", false)

	r := httptest.NewRequest("GET", "/api/v1/proposals/"+id, nil)
	w := servePublicMux(t, "GET", "/api/v1/proposals/{id}", handler.GetProposal(rig.db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous read: got %d: %s", w.Code, w.Body.String())
	}
	got := decodeJSON(t, w)
	if got["doc_text_hidden"] != true {
		t.Fatalf("expected the members-only charter's text hidden, got %v", got["doc_text_hidden"])
	}
	if _, ok := got["base_doc_content"]; ok {
		t.Error("base_doc_content must be withheld with the rest of the charter's text")
	}
}
