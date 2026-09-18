package patchwork

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// timestampLayoutLiteral is internal/clock.Layout, spelled out rather than
// imported: the whole point of this test is to notice a copy of the string
// that isn't going through that package.
const timestampLayoutLiteral = `"2006-01-02T15:04:05.000Z"`

// timestampLayoutRoots are the trees issue #311 measured: 128 raw copies of
// the millisecond layout and 150 uses of time.RFC3339, both writing the same
// column in two shapes that sort wrong against each other within the same
// second ('.' < 'Z'). internal/clock now holds the one constant and the
// Now/Format/Parse helpers that write and read it uniformly.
var timestampLayoutRoots = []string{"internal", "cmd"}

// TestOneTimestampLayout keeps the fix from eroding one file at a time, the
// same way TestNewRecordsAreNotNumbered keeps a numbered ADR from creeping
// back in: a fresh call site is far more likely to copy a literal already
// sitting in the file next to it than to go looking for internal/clock.
//
// Only non-test, non-clock-package .go files are walked. Tests build fixture
// timestamps by hand for reasons that have nothing to do with the storage
// format (deadlines relative to time.Now(), fixed expiries, etc.), and
// guarding them would just make every new test import a package it has no
// production reason to know about. What matters is that nothing which reads
// or writes a real column rolls its own copy of the layout.
func TestOneTimestampLayout(t *testing.T) {
	for _, root := range timestampLayoutRoots {
		root := root
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// internal/clock is the layout's one legitimate home.
			dir := filepath.ToSlash(filepath.Dir(path))
			if dir == "internal/clock" {
				return nil
			}

			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), timestampLayoutLiteral) {
				t.Errorf("%s: contains a raw copy of the timestamp layout literal %s; "+
					"use clock.Now(), clock.Format(t) or clock.Parse(s) from internal/clock instead "+
					"(see issue #311)", path, timestampLayoutLiteral)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
