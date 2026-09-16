package handler

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// findInstanceRoleChecks walks this package's own source and returns, per
// "file.go:Func", how many times that function compares a user's instance
// role against "admin".
//
// It parses rather than greps on purpose. The three clearest statements of
// why this reach is wrong are code *comments* containing the literal
// `user.Role == "admin"` (proposals.go, at CreateProposal, VoteOnProposal and
// ApplyProposal, each explaining a bypass that was deliberately removed). A
// text scan counts those as the thing they argue against, which would make
// the inventory a list of its own refutations.
func findInstanceRoleChecks(t *testing.T) map[string]int {
	t.Helper()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package handler: %v", err)
	}

	found := map[string]int{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			base := path
			if i := strings.LastIndexAny(base, `/\`); i >= 0 {
				base = base[i+1:]
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				key := base + ":" + fn.Name.Name
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if isInstanceRoleCheck(n) {
						found[key]++
					}
					return true
				})
			}
		}
	}
	return found
}

// isInstanceRoleCheck reports whether n compares `user.Role` (or `u.Role`)
// against the literal "admin". Membership roles are a different field on a
// different struct and never match: those read `role == "admin"` off a
// memberships row, which is what makes a patch admin and is not this.
func isInstanceRoleCheck(n ast.Node) bool {
	bin, ok := n.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
		return false
	}
	return (isUserRoleSelector(bin.X) && isAdminLiteral(bin.Y)) ||
		(isUserRoleSelector(bin.Y) && isAdminLiteral(bin.X))
}

func isUserRoleSelector(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Role" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && (ident.Name == "user" || ident.Name == "u")
}

func isAdminLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"admin"`
}

// TestEveryInstanceAdminReachIsDeclared is the guard described in
// docs/adr/115. Every place this package asks whether the caller is an
// instance admin has to be named in instanceAdminReach, with what ADR 115
// says it should end up as.
//
// This is a characterization test: it pins today's behaviour and does not
// assert the ADR's rule, whose implementation is deliberately deferred
// (issue #286). Its job is narrow and worth doing on its own — the reach
// reached thirty sites by being copied from the handler next door, and every
// node route written before #286 lands would copy it again.
//
// Turning it into the enforcement test later is an edit to the declarations,
// not another archaeology pass over the package.
func TestEveryInstanceAdminReachIsDeclared(t *testing.T) {
	found := findInstanceRoleChecks(t)

	var undeclared, stale, miscounted []string

	for key, count := range found {
		rule, ok := instanceAdminReach[key]
		if !ok {
			undeclared = append(undeclared, fmt.Sprintf("%s (%d)", key, count))
			continue
		}
		if rule.count != count {
			miscounted = append(miscounted,
				fmt.Sprintf("%s: declared %d, found %d", key, rule.count, count))
		}
	}
	for key := range instanceAdminReach {
		if _, ok := found[key]; !ok {
			stale = append(stale, key)
		}
	}

	sort.Strings(undeclared)
	sort.Strings(stale)
	sort.Strings(miscounted)

	if len(undeclared) > 0 {
		t.Errorf("instance-admin checks with no entry in instanceAdminReach:\n  %s\n\n"+
			"A new one of these is almost always the reflex docs/adr/115 is about: an instance admin "+
			"holding no role in a patch getting a patch admin's verb because the handler next door does "+
			"it that way. Read docs/adr/115 and instance_admin_reach.go, decide which disposition this "+
			"site has, and declare it. If the answer is reachNone, prefer not writing the check at all.",
			strings.Join(undeclared, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("instanceAdminReach entries with no matching check (site removed or renamed?):\n  %s\n\n"+
			"If issue #286 removed it, delete the entry. If the function was renamed, rename the key.",
			strings.Join(stale, "\n  "))
	}
	if len(miscounted) > 0 {
		t.Errorf("instance-admin check counts changed:\n  %s\n\n"+
			"A function that already had one of these grew or lost one. Update the count once you have "+
			"checked the new one is not a fresh bypass riding in beside a declared one.",
			strings.Join(miscounted, "\n  "))
	}

	// Progress on #286, so the size of the remaining job is visible without
	// re-deriving it. Deliberately not an assertion: the deferral is a
	// decision, not a regression.
	var unsettled int
	for _, rule := range instanceAdminReach {
		if !rule.settled {
			unsettled++
		}
	}
	t.Logf("docs/adr/115: %d of %d declared sites still to change (issue #286)",
		unsettled, len(instanceAdminReach))
}
