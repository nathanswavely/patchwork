// Command releasenotes validates the release-notes/ files described in
// docs/adr/085 and emits the machine-readable release.json that CI attaches
// to a GitHub release.
//
// A release's notes are one file, release-notes/vX.Y.Z.md, written by the
// person cutting the tag. YAML front matter carries the facts an unattended
// updater needs — is this breaking, can its migrations be rolled back, what
// happened in one line — and the body below the fence is prose for a human.
//
// With no flags it validates every notes file in the directory, which is what
// `make release-notes-check` and every CI run do:
//
//	releasenotes
//
// With -tag it additionally insists that this release has a file at all, which
// is the gate that stops a tag publishing an image with nothing said about it:
//
//	releasenotes -tag v0.9.0
//
// With -image and -json it writes the artifact downstream reads, and -body
// writes the prose alone for `gh release create --notes-file`:
//
//	releasenotes -tag v0.9.0 -image ghcr.io/org/repo:0.9.0 \
//	    -json release.json -body release-body.md
//
// Every failure is a non-zero exit with the offending file named, because the
// only place this runs unattended is a pipeline whose log is the error report.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// A version is the git tag verbatim, leading v and all: the filename is
// vX.Y.Z.md so that `release-notes/$GITHUB_REF_NAME.md` is the whole lookup.
// A prerelease suffix is allowed because a -rc tag is still a tag CI will
// publish for; build metadata is not, because it does not survive a Docker
// tag (`+` is not a legal character in one).
var versionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// summaryMax keeps `summary` to something that works as a release title —
// the existing releases read "v0.8.0 — claims complete through setup". A cap
// is what makes "one line" mean anything.
const summaryMax = 120

// knownKeys is closed on purpose. An unrecognised key is far more often a
// typo in an optional field than a deliberate extension, and the tool and the
// files it reads always ship in the same commit, so nothing gains from being
// lenient here.
var knownKeys = map[string]bool{
	"version":                 true,
	"breaking":                true,
	"irreversible_migrations": true,
	"summary":                 true,
}

// Notes is one parsed release-notes file.
type Notes struct {
	Version                string
	Breaking               bool
	IrreversibleMigrations bool
	Summary                string
	Body                   string
}

// Release is the shape of release.json — the whole contract with a fleet
// updater or a self-hoster's cron job. Fields are only ever added.
type Release struct {
	Version                string `json:"version"`
	Image                  string `json:"image"`
	Breaking               bool   `json:"breaking"`
	IrreversibleMigrations bool   `json:"irreversible_migrations"`
	Summary                string `json:"summary"`
}

func main() {
	var (
		dir   = flag.String("dir", "release-notes", "directory holding the vX.Y.Z.md files")
		tag   = flag.String("tag", "", "release tag whose notes must exist (e.g. v0.9.0)")
		image = flag.String("image", "", "published image reference to record in release.json")
		out   = flag.String("json", "", "write release.json here (requires -tag)")
		body  = flag.String("body", "", "write the prose body here, front matter stripped (requires -tag)")
	)
	flag.Parse()

	if err := run(*dir, *tag, *image, *out, *body); err != nil {
		fmt.Fprintln(os.Stderr, "releasenotes:", err)
		os.Exit(1)
	}
}

func run(dir, tag, image, jsonPath, bodyPath string) error {
	if (jsonPath != "" || bodyPath != "") && tag == "" {
		return errors.New("-json and -body need -tag: they describe one release")
	}

	// Everything in the directory is checked on every run, not just the tag
	// being cut. A malformed file is then a red PR rather than a surprise at
	// the moment somebody is trying to ship.
	all, err := checkAll(dir)
	if err != nil {
		return err
	}
	fmt.Printf("release notes: %d file(s) in %s are valid\n", len(all), dir)

	if tag == "" {
		return nil
	}
	if !versionRE.MatchString(tag) {
		return fmt.Errorf("tag %q is not a vX.Y.Z version", tag)
	}

	n, ok := all[tag]
	if !ok {
		return fmt.Errorf("no notes for %s: write %s before pushing the tag (see release-notes/README.md)",
			tag, filepath.Join(dir, tag+".md"))
	}

	if jsonPath != "" {
		if image == "" {
			return errors.New("-json needs -image: release.json names the image it describes")
		}
		buf, err := json.MarshalIndent(Release{
			Version:                n.Version,
			Image:                  image,
			Breaking:               n.Breaking,
			IrreversibleMigrations: n.IrreversibleMigrations,
			Summary:                n.Summary,
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(jsonPath, append(buf, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", jsonPath)
	}

	if bodyPath != "" {
		if err := os.WriteFile(bodyPath, []byte(n.Body+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", bodyPath)
	}

	fmt.Printf("%s: breaking=%v irreversible_migrations=%v — %s\n",
		n.Version, n.Breaking, n.IrreversibleMigrations, n.Summary)
	return nil
}

// checkAll parses every notes file in dir, keyed by version. It reports every
// broken file rather than the first, so one run fixes one round of mistakes.
func checkAll(dir string) (map[string]*Notes, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	notes := map[string]*Notes{}
	var problems []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "README.md" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		n, err := Parse(name, src)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", filepath.Join(dir, name), err))
			continue
		}
		notes[n.Version] = n
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	return notes, nil
}

// Parse reads one notes file. filename carries the version — the file is the
// only place a version is stated, so there is nothing for it to disagree with
// unless somebody restates it in the front matter, which is then checked.
func Parse(filename string, src []byte) (*Notes, error) {
	base := filepath.Base(filename)
	if !strings.HasSuffix(base, ".md") {
		return nil, fmt.Errorf("not a .md file")
	}
	version := strings.TrimSuffix(base, ".md")
	if !versionRE.MatchString(version) {
		return nil, fmt.Errorf("filename must be vX.Y.Z.md, got %q", base)
	}

	fm, body, err := splitFrontMatter(string(src))
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(fm), &raw); err != nil {
		return nil, fmt.Errorf("front matter is not valid YAML: %w", err)
	}
	if raw == nil {
		return nil, errors.New("front matter is empty")
	}
	for k := range raw {
		if !knownKeys[k] {
			return nil, fmt.Errorf("unknown front matter key %q", k)
		}
	}

	n := &Notes{Version: version}

	// A real boolean, not a string that looks like one. yaml.v3 decodes only
	// true/false into a bool, so "true" and yes both land here as strings and
	// are rejected — an updater that gates on this must never read a quoted
	// "false" as falsy by luck of the parser at the other end.
	if n.Breaking, err = requireBool(raw, "breaking"); err != nil {
		return nil, err
	}
	if n.IrreversibleMigrations, err = requireBool(raw, "irreversible_migrations"); err != nil {
		return nil, err
	}

	summary, ok := raw["summary"]
	if !ok {
		return nil, errors.New("missing required key \"summary\"")
	}
	s, ok := summary.(string)
	if !ok {
		return nil, fmt.Errorf("summary must be a string, got %T", summary)
	}
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return nil, errors.New("summary is empty")
	case strings.Contains(s, "\n"):
		return nil, errors.New("summary must be one line")
	case len(s) > summaryMax:
		return nil, fmt.Errorf("summary is %d characters; keep it under %d so it works as a release title", len(s), summaryMax)
	}
	n.Summary = s

	if v, ok := raw["version"]; ok {
		vs, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("version must be a string, got %T", v)
		}
		if vs != version {
			return nil, fmt.Errorf("front matter says version %q but the filename says %q", vs, version)
		}
	}

	n.Body = strings.TrimSpace(body)
	if n.Body == "" {
		return nil, errors.New("body is empty: the release page needs prose a person can read")
	}
	return n, nil
}

func requireBool(raw map[string]any, key string) (bool, error) {
	v, ok := raw[key]
	if !ok {
		return false, fmt.Errorf("missing required key %q", key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be true or false, got %T (%v)", key, v, v)
	}
	return b, nil
}

// splitFrontMatter takes the YAML between the opening --- and the next line
// that is exactly ---, and returns everything after it as the body.
func splitFrontMatter(text string) (fm, body string, err error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", errors.New("must open with a --- front matter fence on line 1")
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return "", "", errors.New("front matter is never closed by a --- line")
}
