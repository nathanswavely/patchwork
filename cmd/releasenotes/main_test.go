package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodNotes = `---
breaking: false
irreversible_migrations: false
summary: claims complete through setup
---

Claiming a patch now runs end to end.

## Upgrading

Nothing to do.
`

func TestParseValidFile(t *testing.T) {
	n, err := Parse("v0.8.0.md", []byte(goodNotes))
	if err != nil {
		t.Fatalf("valid file rejected: %v", err)
	}
	if n.Version != "v0.8.0" {
		t.Errorf("version = %q, want v0.8.0", n.Version)
	}
	if n.Breaking || n.IrreversibleMigrations {
		t.Errorf("flags = %v/%v, want false/false", n.Breaking, n.IrreversibleMigrations)
	}
	if n.Summary != "claims complete through setup" {
		t.Errorf("summary = %q", n.Summary)
	}
	if !strings.HasPrefix(n.Body, "Claiming a patch") {
		t.Errorf("body should start at the prose, got %q", n.Body)
	}
	if strings.Contains(n.Body, "---") || strings.Contains(n.Body, "summary:") {
		t.Errorf("front matter leaked into the body: %q", n.Body)
	}
}

func TestParseAcceptsPrereleaseAndRestatedVersion(t *testing.T) {
	src := "---\nversion: v1.0.0-rc.1\nbreaking: true\nirreversible_migrations: true\nsummary: the flat rewrite\n---\n\nprose\n"
	n, err := Parse("v1.0.0-rc.1.md", []byte(src))
	if err != nil {
		t.Fatalf("prerelease rejected: %v", err)
	}
	if !n.Breaking || !n.IrreversibleMigrations {
		t.Errorf("flags = %v/%v, want true/true", n.Breaking, n.IrreversibleMigrations)
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		src      string
		want     string // substring the error must name
	}{
		{
			name:     "missing breaking",
			filename: "v0.9.0.md",
			src:      "---\nirreversible_migrations: false\nsummary: a release\n---\n\nprose\n",
			want:     `missing required key "breaking"`,
		},
		{
			name:     "missing irreversible_migrations",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nsummary: a release\n---\n\nprose\n",
			want:     `missing required key "irreversible_migrations"`,
		},
		{
			name:     "missing summary",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: false\n---\n\nprose\n",
			want:     `missing required key "summary"`,
		},
		{
			// The whole point of the flag is that a machine reads it. A
			// quoted "false" is truthy in most languages' JSON handling.
			name:     "breaking is a quoted string",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: \"false\"\nirreversible_migrations: false\nsummary: a release\n---\n\nprose\n",
			want:     "breaking must be true or false",
		},
		{
			name:     "breaking is a word",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: maybe\nirreversible_migrations: false\nsummary: a release\n---\n\nprose\n",
			want:     "breaking must be true or false",
		},
		{
			// yaml.v3 follows the YAML 1.2 core schema, where yes is a
			// string. Better to say so than to guess which one was meant.
			name:     "breaking is yes",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: yes\nirreversible_migrations: false\nsummary: a release\n---\n\nprose\n",
			want:     "breaking must be true or false",
		},
		{
			name:     "irreversible_migrations is a number",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: 1\nsummary: a release\n---\n\nprose\n",
			want:     "irreversible_migrations must be true or false",
		},
		{
			name:     "front matter version disagrees with the filename",
			filename: "v0.9.0.md",
			src:      "---\nversion: v0.9.1\nbreaking: false\nirreversible_migrations: false\nsummary: a release\n---\n\nprose\n",
			want:     "front matter says version",
		},
		{
			name:     "filename is not a version",
			filename: "next.md",
			src:      goodNotes,
			want:     "filename must be vX.Y.Z.md",
		},
		{
			name:     "filename has no v prefix",
			filename: "0.9.0.md",
			src:      goodNotes,
			want:     "filename must be vX.Y.Z.md",
		},
		{
			name:     "no front matter at all",
			filename: "v0.9.0.md",
			src:      "# v0.9.0\n\nprose\n",
			want:     "must open with a --- front matter fence",
		},
		{
			name:     "front matter never closes",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: false\nsummary: a release\n\nprose\n",
			want:     "never closed",
		},
		{
			name:     "unknown key",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: false\nsummary: a release\nbraking: true\n---\n\nprose\n",
			want:     `unknown front matter key "braking"`,
		},
		{
			name:     "empty summary",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: false\nsummary: \"  \"\n---\n\nprose\n",
			want:     "summary is empty",
		},
		{
			name:     "summary too long to be a title",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: false\nsummary: " + strings.Repeat("a", summaryMax+1) + "\n---\n\nprose\n",
			want:     "keep it under",
		},
		{
			name:     "empty body",
			filename: "v0.9.0.md",
			src:      "---\nbreaking: false\nirreversible_migrations: false\nsummary: a release\n---\n\n",
			want:     "body is empty",
		},
		{
			name:     "front matter is not a mapping",
			filename: "v0.9.0.md",
			src:      "---\n- breaking\n---\n\nprose\n",
			want:     "not valid YAML",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.filename, []byte(tc.src))
			if err == nil {
				t.Fatalf("expected an error naming %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseHandlesCRLF(t *testing.T) {
	if _, err := Parse("v0.9.0.md", []byte(strings.ReplaceAll(goodNotes, "\n", "\r\n"))); err != nil {
		t.Fatalf("CRLF file rejected: %v", err)
	}
}

// writeDir lays out a release-notes directory for the end-to-end runs.
func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunEmitsReleaseJSON(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"README.md":  "not a release; must be skipped\n",
		"v0.8.0.md":  goodNotes,
		"v0.25.1.md": "---\nbreaking: true\nirreversible_migrations: true\nsummary: the working directory moved\n---\n\nRead this before updating.\n",
	})
	out := filepath.Join(t.TempDir(), "release.json")
	body := filepath.Join(t.TempDir(), "release-body.md")

	err := run(dir, "v0.25.1", "ghcr.io/patchwork-toolkit/patchwork:0.25.1", out, body)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	buf, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("release.json is not JSON: %v", err)
	}
	want := map[string]any{
		"version":                 "v0.25.1",
		"image":                   "ghcr.io/patchwork-toolkit/patchwork:0.25.1",
		"breaking":                true,
		"irreversible_migrations": true,
		"summary":                 "the working directory moved",
	}
	if len(got) != len(want) {
		t.Errorf("release.json has keys %v, want exactly %v", keys(got), keys(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("release.json[%q] = %#v, want %#v", k, got[k], v)
		}
	}
	// booleans, not strings — the field an updater gates on
	if _, ok := got["breaking"].(bool); !ok {
		t.Errorf("breaking is %T in JSON, want bool", got["breaking"])
	}

	prose, err := os.ReadFile(body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prose), "breaking:") {
		t.Errorf("release body carries front matter: %q", prose)
	}
	if !strings.HasPrefix(string(prose), "Read this before updating.") {
		t.Errorf("release body = %q", prose)
	}
}

func TestRunFailsWhenTheTagHasNoNotes(t *testing.T) {
	dir := writeDir(t, map[string]string{"v0.8.0.md": goodNotes})
	err := run(dir, "v0.9.0", "", "", "")
	if err == nil {
		t.Fatal("a tag with no notes must not pass")
	}
	if !strings.Contains(err.Error(), "no notes for v0.9.0") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunFailsOnAnyBrokenFileEvenWithoutATag(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"v0.8.0.md": goodNotes,
		"v0.9.0.md": "---\nbreaking: nope\nirreversible_migrations: false\nsummary: x\n---\n\nprose\n",
	})
	err := run(dir, "", "", "", "")
	if err == nil {
		t.Fatal("a malformed file must fail the check even when no tag is being cut")
	}
	if !strings.Contains(err.Error(), "v0.9.0.md") {
		t.Fatalf("error should name the file, got %q", err)
	}
}

func TestRunRejectsANonVersionTag(t *testing.T) {
	dir := writeDir(t, map[string]string{"v0.8.0.md": goodNotes})
	if err := run(dir, "release-candidate", "", "", ""); err == nil {
		t.Fatal("a tag that is not a version must be rejected")
	}
}

func TestRunNeedsATagToEmit(t *testing.T) {
	dir := writeDir(t, map[string]string{"v0.8.0.md": goodNotes})
	if err := run(dir, "", "img", filepath.Join(t.TempDir(), "release.json"), ""); err == nil {
		t.Fatal("-json without -tag must be rejected")
	}
}

func TestRunNeedsAnImageToEmit(t *testing.T) {
	dir := writeDir(t, map[string]string{"v0.8.0.md": goodNotes})
	if err := run(dir, "v0.8.0", "", filepath.Join(t.TempDir(), "release.json"), ""); err == nil {
		t.Fatal("-json without -image must be rejected")
	}
}

// TestRepoNotesAreValid runs the checker over the directory this repo ships,
// so a notes file added in a PR is validated by `go test ./...` as well as by
// the workflow.
func TestRepoNotesAreValid(t *testing.T) {
	if err := run("../../release-notes", "", "", "", ""); err != nil {
		t.Fatalf("release-notes/ in this repo does not validate: %v", err)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
