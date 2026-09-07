package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Every audit action the seeder writes is one the product actually writes.
//
// The seeded rows carried bare verbs — "create", "update" — while
// LogAuditEvent writes "node.create" and "membership.approve". The admin audit
// log's Action filter offers the real ones, so it matched no seeded row and
// every filter read empty on a demo instance. Nothing failed: the rows
// inserted, the table rendered them, and only someone using the filter would
// notice.
//
// This is the second drift of its kind in this file. The notification
// fixtures had it too, and those are fixed for good by naming the constants
// in internal/notifications — a rename there is now a compile error here.
// Audit actions have no constants to name; they are string literals at each
// call site. So they get a test instead.
func TestSeededAuditActionsExist(t *testing.T) {
	emitted := auditActionsInSource(t)
	if len(emitted) < 50 {
		t.Fatalf("only %d audit actions found in the tree; the source scan has gone blind", len(emitted))
	}

	for _, a := range seededAuditActions {
		if !emitted[a.action] {
			t.Errorf("seeded audit action %q is not written anywhere in the product\n"+
				"      A demo instance shows it in the audit log, and the Action filter\n"+
				"      never matches it. Use the action the handler logs, or drop the row.",
				a.action)
		}
	}

	// The entity type travels with the action, and the filter and the detail
	// view both read it. A real action against the wrong entity type is a row
	// that could not have happened.
	for _, a := range seededAuditActions {
		if !emitted[a.action] {
			continue // already reported above
		}
		if want, ok := auditEntityFor(t, a.action); ok && want != a.entityType {
			t.Errorf("seeded %q is logged against entity type %q, not %q",
				a.action, want, a.entityType)
		}
	}
}

// auditActionsInSource collects the action literal of every LogAuditEvent call
// in the tree. go/parser rather than a grep, so a call split across lines is
// still one call.
func auditActionsInSource(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	forEachLogAuditEvent(t, func(action, entityType string) { out[action] = true })
	return out
}

// auditEntityFor reports the entity type an action is logged against, when the
// tree agrees on exactly one.
func auditEntityFor(t *testing.T, action string) (string, bool) {
	t.Helper()
	seen := map[string]bool{}
	forEachLogAuditEvent(t, func(a, entityType string) {
		if a == action && entityType != "" {
			seen[entityType] = true
		}
	})
	if len(seen) != 1 {
		return "", false // logged against several, or none we could read
	}
	for e := range seen {
		return e, true
	}
	return "", false
}

func forEachLogAuditEvent(t *testing.T, visit func(action, entityType string)) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected the repo root at %s: %v", root, err)
	}

	fset := token.NewFileSet()
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Errorf("parse %s: %v", path, perr)
				return nil
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "LogAuditEvent" || len(call.Args) < 4 {
					return true
				}
				// LogAuditEvent(db, userID, action, entityType, ...)
				action, aok := stringLit(call.Args[2])
				if !aok {
					return true
				}
				entity, _ := stringLit(call.Args[3])
				visit(action, entity)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}
