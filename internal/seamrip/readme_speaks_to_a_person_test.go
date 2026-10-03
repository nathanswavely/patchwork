package seamrip

import (
	"strings"
	"testing"
)

// F-103. The archive's only instruction for using it was a Go command.
//
// A printmaker took a member seamrip so her press could start again
// somewhere else, opened the zip, and found `go run ./cmd/import -db
// ./new-patchwork.db -in ./export/`. "I fold zines. I can't do that, and nor
// can Tam or Gus. What I'd want is a sentence saying 'any Patchwork can read
// this — give it to whoever sets up the new one.'"
//
// The person who takes a seamrip is, by design, not an administrator: docs/adr/012
// affordance 2 exists so a member does not have to wait for one. So the
// README opens for that reader and keeps the command for the person who
// will actually run it.

func TestMemberReadme_TellsANonTechnicalPersonWhatToDo(t *testing.T) {
	// The sentence she asked for, in substance: any Patchwork reads it, and
	// it goes to whoever sets the new one up.
	for _, want := range []string{
		"Any Patchwork can read it",
		"whoever sets up the new quilt",
		"You do not\nneed to do anything technical with it yourself",
	} {
		if !strings.Contains(MemberReadmeText, want) {
			t.Errorf("the README does not say %q", want)
		}
	}
}

// The plain part comes first. A reader who meets the command before the
// sentence has already decided the file is not for them.
func TestMemberReadme_PutsThePersonBeforeTheCommand(t *testing.T) {
	person := strings.Index(MemberReadmeText, "WHAT TO DO WITH THIS FILE")
	cmd := strings.Index(MemberReadmeText, "go run ./cmd/import")
	if person < 0 || cmd < 0 {
		t.Fatal("expected both a plain section and the import command")
	}
	if person > cmd {
		t.Error("the import command comes before the sentence a non-technical reader needs")
	}
}

// And the command is still there, under its own heading, because somebody
// does have to run it.
func TestMemberReadme_KeepsTheCommandForWhoeverRunsIt(t *testing.T) {
	if !strings.Contains(MemberReadmeText, "FOR WHOEVER SETS UP THE NEW QUILT") {
		t.Error("the technical half is not labelled for the person it is for")
	}
	if !strings.Contains(MemberReadmeText, "go run ./cmd/import -db ./new-patchwork.db -in ./that-folder/") {
		t.Error("the import command is gone")
	}
	// -in is a directory and ./export/ is only what `make export` happens
	// to call its output, so a reader who unzipped this into Downloads was
	// being shown a path that is not theirs.
	if !strings.Contains(MemberReadmeText, "the folder you unzipped it into") {
		t.Error("the README does not say what -in points at")
	}
	if !strings.Contains(MemberReadmeText, "id_map.json") {
		t.Error("the id mapping note is gone")
	}
}

// It must not read as something that changes this quilt. She spent half a
// minute wondering whether the feature removed her.
func TestMemberReadme_SaysItTakesNothingAway(t *testing.T) {
	if !strings.Contains(MemberReadmeText, "no use to\nanyone as a way into this quilt") {
		t.Errorf("the README does not say the file is harmless to hold: %q", MemberReadmeText)
	}
}
