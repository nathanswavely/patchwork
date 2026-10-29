package copystats

import (
	"os"
	"path/filepath"
	"testing"
)

const fixture = `{"entries": [
  {"words": 2,  "tier": "label",  "status": "ai-fine",    "occurrences": [{"file": "web/src/pages/About.svelte", "kind": "markup"}]},
  {"words": 3,  "tier": "label",  "status": "unreviewed", "occurrences": [{"file": "web/src/x.svelte", "kind": "markup"}]},
  {"words": 18, "tier": "prose",  "status": "unreviewed", "occurrences": [{"file": "internal/handler/a.go", "kind": "go-error-body"}]},
  {"words": 20, "tier": "prose",  "status": "human",      "occurrences": [{"file": "web/src/pages/About.svelte", "kind": "markup"}]},
  {"words": 30, "tier": "prose",  "status": "ai-fine",    "occurrences": [{"file": "web/src/pages/About.svelte", "kind": "markup"}, {"file": "web/src/pages/Label.svelte", "kind": "markup"}]},
  {"words": 17, "tier": "prose",  "status": "ai-revised", "occurrences": [{"file": "web/src/y.svelte", "kind": "markup"}]},
  {"words": 25, "tier": "prose",  "status": "rewritten",  "occurrences": [{"file": "internal/handler/legal_defaults.go", "kind": "md-block"}]},
  {"words": 19, "tier": "prose",  "status": "not-copy",   "occurrences": [{"file": "web/src/z.js", "kind": "js-string"}]},
  {"words": 16, "tier": "prose",  "status": "someday",    "occurrences": [{"file": "web/src/z.js", "kind": "js-string"}]}
]}`

func TestSummaryClassesAndBuckets(t *testing.T) {
	s, err := FromLedger([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}

	// Two labels plus a long error body: interface text is decided by what
	// a string does before how long it is.
	if s.Interface != (Strings{Reviewed: 1, Total: 3}) {
		t.Errorf("interface = %+v, want 1 of 3 reviewed", s.Interface)
	}

	// ai-revised is a model's rewrite taken unchanged, so it is drafted. A
	// rewrite not yet applied is still the draft in source. A status this
	// code does not know is never counted as a person's work.
	want := Words{Human: 20, Drafted: 30 + 17, Unreviewed: 25 + 16}
	if s.Writing != want {
		t.Errorf("writing = %+v, want %+v", s.Writing, want)
	}

	// Not-copy is left out of the totals and counted where it can be seen.
	if s.NotCopy != 1 {
		t.Errorf("not_copy = %d, want 1", s.NotCopy)
	}

	pages := map[string]Words{}
	for _, p := range s.Pages {
		pages[p.ID] = p.Words
	}
	// About's label is interface text and stays off its writing row; the
	// paragraph shared with the Label counts once on each page.
	if pages["about"] != (Words{Human: 20, Drafted: 30}) {
		t.Errorf("about = %+v", pages["about"])
	}
	if pages["label"] != (Words{Drafted: 30}) {
		t.Errorf("label = %+v", pages["label"])
	}
	if pages["legal"] != (Words{Unreviewed: 25}) {
		t.Errorf("legal = %+v", pages["legal"])
	}
}

// The embedded ledger is the one CI reviews, so it must parse, and every
// row the page shows must be backed by strings the ledger can see.
func TestCurrentLedgerParses(t *testing.T) {
	s, err := Current()
	if err != nil {
		t.Fatalf("embedded ledger: %v", err)
	}
	if s.Interface.Total == 0 || s.Writing.Human+s.Writing.Drafted+s.Writing.Unreviewed == 0 {
		t.Fatalf("embedded ledger produced empty figures: %+v", s)
	}
	if len(s.Pages) != len(VisitorPages) {
		t.Fatalf("got %d page rows, want %d", len(s.Pages), len(VisitorPages))
	}
}

// A renamed or moved page would otherwise keep a row of zeros forever.
func TestVisitorPageFilesExist(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, p := range VisitorPages {
		for _, f := range p.Files {
			if _, err := os.Stat(filepath.Join(root, f)); err != nil {
				t.Errorf("visitor page %q lists %s: %v", p.ID, f, err)
			}
		}
	}
}
