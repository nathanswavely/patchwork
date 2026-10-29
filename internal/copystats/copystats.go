// Package copystats turns the copy ledger into the figures the AI page
// publishes (docs/adr/2026-10-20-the-site-says-where-ai-is.md, decision 5).
//
// A single percentage of strings would count "Save" the same as a paragraph
// of the privacy policy, so the ledger is reported in two classes:
//
//   - interface text (buttons, labels, field hints, error bodies), counted in
//     strings, as reviewed or not, since authorship is not a meaningful
//     question for a button;
//   - writing (paragraphs and documents), counted in words, as written by a
//     person, drafted with AI and kept, or not reviewed yet.
//
// Strings marked not-copy are left out of both and counted on their own, so
// excluding text can never quietly improve a figure.
package copystats

import (
	"encoding/json"
	"strings"
	"sync"

	copyledger "github.com/patchwork-toolkit/patchwork/copy"
)

type occurrence struct {
	File string `json:"file"`
	Kind string `json:"kind"`
}

type entry struct {
	Words       int          `json:"words"`
	Tier        string       `json:"tier"`
	Status      string       `json:"status"`
	Occurrences []occurrence `json:"occurrences"`
}

// Words splits a class of writing by who wrote it. Unreviewed includes a
// rewrite that has not been applied yet, because until it is, the source
// still holds the draft.
type Words struct {
	Human      int `json:"human"`
	Drafted    int `json:"drafted"`
	Unreviewed int `json:"unreviewed"`
}

// Strings counts interface text by whether a person has looked at it.
type Strings struct {
	Reviewed int `json:"reviewed"`
	Total    int `json:"total"`
}

// Page is one visitor page's writing, split the same way as the totals.
type Page struct {
	ID string `json:"id"`
	Words
}

type Summary struct {
	Interface Strings `json:"interface"`
	Writing   Words   `json:"writing"`
	NotCopy   int     `json:"not_copy"`
	Pages     []Page  `json:"pages"`
}

// VisitorPages names the pages a visitor reads and the source files that
// hold their words. The AI page gives each a row; admin-only screens count
// toward the totals and get none, since no visitor sees them. The display
// names live in the page component, where the copy ledger can see them.
// TestVisitorPageFilesExist keeps every path real.
var VisitorPages = []struct {
	ID    string
	Files []string
}{
	{"lining", []string{"internal/governance/lining.go"}},
	{"charters", []string{"internal/governance/defaults.go"}},
	{"about", []string{"web/src/pages/About.svelte"}},
	{"label", []string{"web/src/pages/Label.svelte"}},
	{"legal", []string{"internal/handler/legal_defaults.go"}},
	{"ai", []string{"internal/handler/ai_defaults.go", "web/src/pages/AiPage.svelte"}},
}

// isInterface classes an entry by what the string does first and by its
// length second, so a 16-word error message is interface text and a
// 14-word sentence on About is not.
func isInterface(e entry) bool {
	if len(e.Occurrences) > 0 {
		k := e.Occurrences[0].Kind
		if strings.HasPrefix(k, "attr:") || k == "go-error-body" || k == "md-heading" {
			return true
		}
	}
	return e.Tier != "prose"
}

// add counts n words under the bucket a status belongs to. An unknown
// status counts as unreviewed: a status this code has not heard of must
// never be read as a person's work.
func (w *Words) add(status string, n int) {
	switch status {
	case "human":
		w.Human += n
	case "ai-fine", "ai-revised":
		w.Drafted += n
	default:
		w.Unreviewed += n
	}
}

func reviewed(status string) bool {
	return status == "human" || status == "ai-fine" || status == "ai-revised"
}

// FromLedger computes the summary from a ledger's JSON.
func FromLedger(data []byte) (Summary, error) {
	var l struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(data, &l); err != nil {
		return Summary{}, err
	}

	s := Summary{Pages: make([]Page, len(VisitorPages))}
	pageOf := map[string][]int{}
	for i, p := range VisitorPages {
		s.Pages[i].ID = p.ID
		for _, f := range p.Files {
			pageOf[f] = append(pageOf[f], i)
		}
	}

	for _, e := range l.Entries {
		if e.Status == "not-copy" {
			s.NotCopy++
			continue
		}
		if isInterface(e) {
			s.Interface.Total++
			if reviewed(e.Status) {
				s.Interface.Reviewed++
			}
			continue
		}
		s.Writing.add(e.Status, e.Words)

		// One string can appear on two pages; it counts once on each,
		// never twice on one.
		seen := map[int]bool{}
		for _, o := range e.Occurrences {
			for _, i := range pageOf[o.File] {
				if !seen[i] {
					seen[i] = true
					s.Pages[i].add(e.Status, e.Words)
				}
			}
		}
	}
	return s, nil
}

var (
	once    sync.Once
	current Summary
	err     error
)

// Current is the summary of the ledger this binary was built with,
// computed once.
func Current() (Summary, error) {
	once.Do(func() { current, err = FromLedger(copyledger.JSON) })
	return current, err
}
