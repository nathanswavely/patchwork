package patchwork

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A hand-built JSON literal assembled with fmt.Sprintf works only as long as
// every %s/%q lands a value that is already known-safe (allowlisted, a fixed
// set of states, a %q-escaped Go string that happens not to diverge from
// JSON escaping). None of that is enforced at the call site, so the danger
// is not today's call sites — the reviewer who added this check found them
// all validated — it is the first one that isn't: a join message, a display
// name, a tag name, or any other free-text field spliced into one of these
// literals produces an unescaped quote and an audit_log/error row that stops
// parsing. See docs/adr/2026-09-16-a-name-nobody-has-to-ask-for.md for the
// sibling problem this project takes the same stance on — catch the shape of
// the mistake at build time rather than trust every future call site to get
// it right by hand.
//
// auth.LogAuditEventJSON and the handler package's writeJSONError/
// writeJSONStatus helpers marshal a Go value instead, so this stays at zero.
var handBuiltJSONLiteral = regexp.MustCompile("Sprintf\\(\\s*`\\{")

// TestNoHandBuiltJSONPayloads walks every non-test Go file under internal/
// and cmd/ and fails on a new fmt.Sprintf call whose format string is a raw
// string literal beginning with `{` — the shape of a hand-assembled JSON
// object. Test files are exempt: their fixtures are allowed to stay as they
// are (see docs/adr and the third-party review behind this test, filed as
// GitHub issue #314).
func TestNoHandBuiltJSONPayloads(t *testing.T) {
	roots := []string{"internal", "cmd"}
	for _, root := range roots {
		root := root
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			content := string(data)
			for _, loc := range handBuiltJSONLiteral.FindAllStringIndex(content, -1) {
				line := 1 + strings.Count(content[:loc[0]], "\n")
				t.Errorf("%s:%d: fmt.Sprintf builds a JSON literal by hand; marshal a "+
					"map[string]any (or a struct) instead — see auth.LogAuditEventJSON "+
					"and internal/handler's writeJSONError/writeJSONStatus", path, line)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}
