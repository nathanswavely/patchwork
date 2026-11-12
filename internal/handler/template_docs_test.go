package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// docs/adr/2026-11-11-a-template-gives-the-patch-what-its-preview-lists.md

func templateRig(t *testing.T) (*database.DB, string, *http.ServeMux) {
	t.Helper()
	db := setupTestDB(t)
	oldDir := governance.GetDataDir()
	tmp := t.TempDir()
	governance.SetDataDir(tmp)
	t.Cleanup(func() { governance.SetDataDir(oldDir) })
	if err := governance.InitInstanceRepo(tmp); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}
	_, token := createTestUser(t, db, "founder", "member")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	mux.HandleFunc("GET /api/v1/templates/{id}", handler.GetTemplate())
	return db, token, mux
}

func createFromTemplate(t *testing.T, mux *http.ServeMux, token string, body map[string]string) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, authedRequest("POST", "/api/v1/nodes", body, token))
	var out map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return w.Code, out
}

type docRow struct {
	title, body, kind, visibility string
}

func patchDocRows(t *testing.T, db *database.DB, nodeID string) []docRow {
	t.Helper()
	rows, err := db.Query(`SELECT title, body, kind, visibility FROM governance_docs WHERE node_id = ? ORDER BY created_at, rowid`, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []docRow
	for rows.Next() {
		var d docRow
		if err := rows.Scan(&d.title, &d.body, &d.kind, &d.visibility); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

// The preview listed six documents for Formal and the patch got one, because
// the fork wrote five of them into git and nothing wrote them into the table
// every read path lists (F-018). Every template, every document: a row, with
// the title the preview names, members-only beside a public lining, and the
// git mirror holding exactly the row's body.
func TestEveryTemplateDocumentIsARowFromBirth(t *testing.T) {
	for _, info := range governance.TemplateList() {
		t.Run(info.ID, func(t *testing.T) {
			db, token, mux := templateRig(t)
			code, node := createFromTemplate(t, mux, token, map[string]string{
				"name": "From " + info.Name, "template": info.ID, "membership_policy": "invite_only",
			})
			if code != http.StatusCreated {
				t.Fatalf("create: %d %v", code, node)
			}
			nodeID := node["id"].(string)

			docs := patchDocRows(t, db, nodeID)
			if len(docs) != len(info.Documents) {
				t.Fatalf("preview promises %d documents, patch has %d", len(info.Documents), len(docs))
			}
			for i, d := range docs {
				if got := governance.Filename(d.title); got != info.Documents[i] {
					t.Errorf("document %d: row %q is %s, preview lists %s", i, d.title, got, info.Documents[i])
				}
				if i == 0 {
					if d.kind != "lining" || d.visibility != "public" {
						t.Errorf("the lining must stay a public lining, got kind=%s visibility=%s", d.kind, d.visibility)
					}
					continue
				}
				if d.kind != "charter" || d.visibility != "members" {
					t.Errorf("%s: want a members-only charter (docs/adr/036), got kind=%s visibility=%s", d.title, d.kind, d.visibility)
				}
				if strings.HasPrefix(d.body, "# ") {
					t.Errorf("%s: the body repeats its title as a heading; the title lives on the row", d.title)
				}
				inGit, err := governance.GetDocument(governance.GetDataDir(), nodeID, governance.Filename(d.title))
				if err != nil {
					t.Errorf("%s: not in the repo: %v", d.title, err)
				} else if inGit != d.body {
					t.Errorf("%s: the git mirror and the row differ", d.title)
				}
			}
		})
	}
}

// An unnamed or unknown template has always forked casual; its documents
// must come from the same template as its rules file.
func TestAnUnknownTemplateGetsCasualsDocumentsToo(t *testing.T) {
	db, token, mux := templateRig(t)
	code, node := createFromTemplate(t, mux, token, map[string]string{"name": "No template", "membership_policy": "open"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, node)
	}
	docs := patchDocRows(t, db, node["id"].(string))
	if len(docs) != 2 || docs[1].title != "Operating Agreement" {
		t.Fatalf("want the lining and casual's Operating Agreement, got %+v", docs)
	}
}

// The preview is the list the patch gets, in the order it gets them, with
// the bodies it gets. It walked a map before, so it shuffled them too.
func TestThePreviewListsWhatThePatchGets(t *testing.T) {
	_, _, mux := templateRig(t)
	for _, info := range governance.TemplateList() {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/templates/"+info.ID, nil))
		var resp struct {
			Template  governance.TemplateInfo `json:"template"`
			Documents []struct {
				Filename string `json:"filename"`
				Title    string `json:"title"`
				Content  string `json:"content"`
			} `json:"documents"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if len(resp.Documents) != len(info.Documents) {
			t.Fatalf("%s: preview shows %d documents, template lists %d", info.ID, len(resp.Documents), len(info.Documents))
		}
		want := map[string]string{governance.DefaultLiningTitle: governance.DefaultLiningBody}
		for _, d := range governance.TemplateDocuments(info.ID) {
			want[d.Title] = d.Body
		}
		for i, d := range resp.Documents {
			if d.Filename != info.Documents[i] || governance.Filename(d.Title) != d.Filename {
				t.Errorf("%s doc %d: %s / %q, want %s", info.ID, i, d.Filename, d.Title, info.Documents[i])
			}
			if want[d.Title] != d.Content {
				t.Errorf("%s: the preview of %s is not the text the patch gets", info.ID, d.Title)
			}
		}
		if resp.Template.Elsewhere == "" {
			t.Errorf("%s: the card does not say decisions can be made at a meeting", info.ID)
		}
	}
}

// The create form carries the card text the server serves. Two copies are
// the price of a form that renders before any request returns; this keeps
// them one text.
func TestTemplateCardsMatchTheServer(t *testing.T) {
	src, err := os.ReadFile("../../web/src/pages/PatchForm.svelte")
	if err != nil {
		t.Fatal(err)
	}
	form := string(src)
	for _, info := range governance.TemplateList() {
		for field, text := range map[string]string{
			"description": info.Description, "leadership": info.Leadership,
			"best_for": info.BestFor, "elsewhere": info.Elsewhere,
		} {
			if !strings.Contains(form, "'"+text+"'") {
				t.Errorf("%s %s: PatchForm.svelte does not carry %q", info.ID, field, text)
			}
		}
		// A head count on a card routed real organisations to the wrong
		// mechanics (F-032); who decides is what a card says now.
		if strings.Contains(info.BestFor, "people") {
			t.Errorf("%s: the card names a group size again: %q", info.ID, info.BestFor)
		}
	}
}

// Claims a template document made about the product that the product does
// not keep, found by auditing each one against the mechanics. Each is a
// sentence a founder would have relied on. If one of these comes back, the
// mechanic it describes has to come with it.
func TestTemplateDocumentsMakeNoClaimTheCodeDoesNotKeep(t *testing.T) {
	refuted := map[string]string{
		"ranked-choice":                             "elections use approval voting",
		"Ranked-choice":                             "elections use approval voting",
		"up to 7 admins":                            "max_admins caps nothing; the council has the seats it has set up",
		"close 7 days before":                       "voting opens when the 14-day nomination window closes",
		"Campaign/discussion period":                "there is no campaign phase between nominations and voting",
		"three longest-tenured":                     "an emptied elected council opens an election; it promotes nobody",
		"emergency election":                        "there is no compressed emergency election",
		"retained for 30 days":                      "a member who rejoins starts as a new member",
		"signatures from at least 10%":              "there is no petition or recall mechanic",
		"supermajority (2/3) with 50%":              "Formal's amendment threshold is consensus, with no fallback",
		"emporary suspension":                       "a removal has no end date; an admin reverses it",
		"majority vote with 50% quorum":             "Formal decides by consensus",
		"Anyone can join. This patch is open.":      "who can join is the creation form's answer, not the template's",
		"Membership requires approval":              "who can join is the creation form's answer, not the template's",
		"A majority of active members must approve": "a vote passes on the votes cast, with quorum, not on a majority of all members",
	}
	for _, tmpl := range governance.ValidTemplates {
		for _, d := range governance.TemplateDocuments(tmpl) {
			for phrase, why := range refuted {
				if strings.Contains(d.Body, phrase) {
					t.Errorf("%s / %s says %q, but %s", tmpl, d.Title, phrase, why)
				}
			}
			if strings.Contains(d.Body, "—") {
				t.Errorf("%s / %s carries an em dash", tmpl, d.Title)
			}
		}
	}
}

// docs/adr/2026-11-11-a-patch-is-asked-its-timezone-when-it-is-made.md
func TestCreationTakesTheTimezoneItIsGiven(t *testing.T) {
	db, token, mux := templateRig(t)

	code, node := createFromTemplate(t, mux, token, map[string]string{
		"name": "Lancaster Choir", "template": "minimal", "membership_policy": "invite_only",
		"timezone": "America/New_York",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, node)
	}
	if node["timezone"] != "America/New_York" {
		t.Errorf("created patch answers timezone %v", node["timezone"])
	}

	// Unanswered still follows the quilt: NULL, not "UTC" written down.
	code, node = createFromTemplate(t, mux, token, map[string]string{
		"name": "No Zone", "template": "minimal", "membership_policy": "invite_only",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, node)
	}
	var stored *string
	if err := db.QueryRow(`SELECT timezone FROM nodes WHERE id = ?`, node["id"]).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("an unanswered timezone was stored as %q; it should follow the quilt", *stored)
	}

	// The same refusals Patch Settings gives: a fixed offset is not a place.
	for _, bad := range []string{"EST", "Not/AZone"} {
		code, out := createFromTemplate(t, mux, token, map[string]string{
			"name": "Bad " + bad, "template": "minimal", "membership_policy": "invite_only", "timezone": bad,
		})
		if code != http.StatusBadRequest {
			t.Errorf("timezone %q: want 400, got %d %v", bad, code, out)
		}
	}
}
