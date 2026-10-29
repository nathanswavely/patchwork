package handler

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	copyledger "github.com/patchwork-toolkit/patchwork/copy"
)

// These tests hold the AI page's claims true
// (docs/adr/2026-10-20-the-site-says-where-ai-is.md, decision 3). Each one
// fails the build when the code moves and the page has not.

const repoRoot = "../.."

// outboundCall matches the ways Go code here can open a connection to
// another machine: the SSRF-guarded client every fetch goes through, a
// hand-built client, the package-level shortcuts, mail, raw dials and DNS.
var outboundCall = regexp.MustCompile(`safehttp\.|&http\.Client\{|http\.DefaultClient|http\.(Get|Post|Head|PostForm)\(|smtp\.(Dial|SendMail)|net\.Dial|tls\.Dial|net\.Lookup|net\.DefaultResolver`)

// notTheRunningSite are the outbound callers the page does not list, each
// with why: they are not something the site does to or for a visitor.
var notTheRunningSite = map[string]string{
	"internal/safehttp/safehttp.go":       "the guarded client itself; its callers are what the page lists",
	"cmd/patchwork/healthcheck.go":        "the container healthcheck, which calls this server on loopback",
	"cmd/patchwork/verify_attestation.go": "a command an operator runs by hand to check another quilt's attestation",
}

func TestEveryOutboundCallerIsOnTheAIPage(t *testing.T) {
	listed := map[string]bool{}
	for _, o := range aiPageOutbound {
		for _, f := range o.Files {
			listed[f] = true
		}
	}

	found := map[string]bool{}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !outboundCall.Match(src) {
				return nil
			}
			rel, _ := filepath.Rel(repoRoot, path)
			rel = filepath.ToSlash(rel)
			found[rel] = true
			if !listed[rel] && notTheRunningSite[rel] == "" {
				t.Errorf("%s reaches another machine and the AI page does not say so: add it to aiPageOutbound with what it talks to, or to notTheRunningSite with why it is not the site", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A listed file that no longer reaches anything is a claim gone stale.
	for f := range listed {
		if !found[f] {
			t.Errorf("aiPageOutbound lists %s, which no longer makes an outbound call", f)
		}
	}
}

// aiServicePackages are the client libraries of model providers and the
// frameworks that wrap them, matched as dependency names.
var aiServicePackages = []string{
	"openai", "anthropic", "@anthropic-ai/", "google.golang.org/genai", "generative-ai",
	"@google/genai", "cohere", "mistral", "ollama", "langchain", "llamaindex",
	"huggingface", "replicate", "@ai-sdk/", "together-ai", "groq",
}

func TestNoAIServiceDependency(t *testing.T) {
	gomod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}

	var deps []string
	for _, line := range strings.Split(string(gomod), "\n") {
		if f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "require ")); len(f) >= 2 && strings.Contains(f[0], ".") {
			deps = append(deps, f[0])
		}
	}
	for name := range pkg.Dependencies {
		deps = append(deps, name)
	}
	for name := range pkg.DevDependencies {
		deps = append(deps, name)
	}
	if len(deps) < 10 {
		t.Fatalf("read only %d dependencies; the parser has stopped seeing them", len(deps))
	}

	for _, d := range deps {
		lower := strings.ToLower(d)
		for _, bad := range aiServicePackages {
			if strings.Contains(lower, bad) {
				t.Errorf("dependency %s looks like a model provider's SDK (%q); the AI page says nothing here calls a model", d, bad)
			}
		}
	}
}

// The page's own words must be a person's (decision 6). This fails until
// every string the copy ledger finds only in the page's two source files is
// recorded as human, which is the point: the page cannot ship as drafted.
//
// A string the page shares with other screens ("Failed to load", "The
// Label") was decided where it first appeared. Demanding it become human
// here would ask somebody to claim words they did not write, which is the
// one thing the ledger exists to prevent.
func TestAIPageWordsAreAPersons(t *testing.T) {
	pageFiles := map[string]bool{
		"internal/handler/ai_defaults.go": true,
		"web/src/pages/AiPage.svelte":     true,
	}
	var l struct {
		Entries []struct {
			ID          string `json:"id"`
			Text        string `json:"text"`
			Status      string `json:"status"`
			Occurrences []struct {
				File string `json:"file"`
			} `json:"occurrences"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(copyledger.JSON, &l); err != nil {
		t.Fatal(err)
	}

	seen := 0
	for _, e := range l.Entries {
		onlyHere := len(e.Occurrences) > 0
		for _, o := range e.Occurrences {
			if !pageFiles[o.File] {
				onlyHere = false
			}
		}
		if !onlyHere {
			continue
		}
		seen++
		if e.Status != "human" {
			text := e.Text
			if len(text) > 60 {
				text = text[:60] + "…"
			}
			t.Errorf("%s:%s is %s, not a person's: %q", e.Occurrences[0].File, e.ID, e.Status, text)
		}
	}
	if seen == 0 {
		t.Fatal("the copy ledger has no strings from the AI page; run make copy-sync")
	}
}
