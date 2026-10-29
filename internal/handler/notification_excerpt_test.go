package handler

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A proposal or comment notification carries an excerpt, not the whole text,
// and the cut counts characters: slicing bytes could end inside a multi-byte
// one and store invalid UTF-8 in the row.
func TestExcerptCutsOnCharacters(t *testing.T) {
	if got := excerpt("  short  ", 200); got != "short" {
		t.Errorf("short text: got %q", got)
	}

	long := strings.Repeat("é", 250)
	got := excerpt(long, 200)
	if !utf8.ValidString(got) {
		t.Fatalf("excerpt is not valid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 201 {
		t.Errorf("expected 200 characters plus an ellipsis, got %d", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected a trailing ellipsis, got %q", got)
	}
}
