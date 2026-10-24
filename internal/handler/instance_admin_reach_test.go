package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
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
//
// And it type-checks rather than matching variable names. The first version
// recognised `user.Role` and `u.Role` and nothing else, so `viewer.Role` in
// writeNodeNotFound sat outside the inventory the whole time it existed. The
// question is not what the variable is called but whether it is a
// *model.User, and only the type checker can answer that without also
// catching a membership row's Role, which is what makes a patch admin and is
// not this.
func findInstanceRoleChecks(t *testing.T) map[string]int {
	t.Helper()

	files, info := typeCheckThisPackage(t)

	found := map[string]int{}
	for name, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := name + ":" + fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if isInstanceRoleCheck(n, info) {
					found[key]++
				}
				return true
			})
		}
	}
	return found
}

// typeCheckThisPackage parses and type-checks package handler's non-test
// files. Dependencies are imported from the compiler's export data, which
// `go list -export` reads out of the build cache, rather than from source:
// type-checking every dependency from source takes twenty seconds, this
// takes about one.
func typeCheckThisPackage(t *testing.T) (map[string]*ast.File, *types.Info) {
	t.Helper()

	out, err := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Export,DepOnly", ".").Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go list -export: %v\n%s", err, stderr)
	}
	exports := map[string]string{}
	var self string
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p struct {
			ImportPath, Export string
			DepOnly            bool
		}
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}
		exports[p.ImportPath] = p.Export
		if !p.DepOnly {
			self = p.ImportPath
		}
	}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package handler: %v", err)
	}
	files := map[string]*ast.File{}
	var list []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files[name] = file
		list = append(list, file)
	}

	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		export, ok := exports[path]
		if !ok || export == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(export)
	})}
	if _, err := conf.Check(self, fset, list, info); err != nil {
		t.Fatalf("type-checking package handler: %v", err)
	}
	return files, info
}

// isInstanceRoleCheck reports whether n compares a *model.User's Role
// against the literal "admin".
func isInstanceRoleCheck(n ast.Node, info *types.Info) bool {
	bin, ok := n.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
		return false
	}
	return (isUserRoleSelector(bin.X, info) && isAdminLiteral(bin.Y)) ||
		(isUserRoleSelector(bin.Y, info) && isAdminLiteral(bin.X))
}

func isUserRoleSelector(e ast.Expr, info *types.Info) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Role" {
		return false
	}
	typ := info.TypeOf(sel.X)
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "User" && obj.Pkg() != nil && strings.HasSuffix(obj.Pkg().Path(), "/internal/model")
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
