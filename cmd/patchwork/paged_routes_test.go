package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The frontend's paging guard (web/src/test/every-paged-read-pages.test.js)
// needs to know which endpoints are paged, and it cannot ask Go. So the list
// sits beside it as JSON, and this test is what stops that list going stale:
// it finds every handler in internal/handler that calls
// parsePaginationParams, finds the GET routes that serve those handlers in
// routes.go, and requires the JSON to name exactly that set.
//
// A new paged handler fails here until it is on the list, and once it is on
// the list the frontend guard asks every caller of it to follow next_cursor,
// narrow the question, or say why not. That is the whole chain: the mistake
// this exists to stop is a screen reading one page of a paged list and
// treating it as the whole thing, and it was made on eight screens at once.

const pagedEndpointsFile = "../../web/src/test/paged-endpoints.json"

// pagedHandlers names every function in internal/handler whose body calls
// parsePaginationParams.
func pagedHandlers(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../../internal/handler", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse internal/handler: %v", err)
	}
	found := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Name.Name == "parsePaginationParams" {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "parsePaginationParams" {
							found[fn.Name.Name] = true
						}
					}
					return true
				})
			}
		}
	}
	return found
}

// pagedRoutes reads routes.go and returns "METHOD /pattern" for every
// registration whose handler expression mentions handler.X for a paged X.
func pagedRoutes(t *testing.T, paged map[string]bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "routes.go", nil, 0)
	if err != nil {
		t.Fatalf("parse routes.go: %v", err)
	}
	seen := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "handleFunc" && sel.Sel.Name != "handle") {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil || !strings.HasPrefix(pattern, "GET ") {
			return true
		}
		ast.Inspect(call.Args[1], func(m ast.Node) bool {
			if s, ok := m.(*ast.SelectorExpr); ok {
				if pkg, ok := s.X.(*ast.Ident); ok && pkg.Name == "handler" && paged[s.Sel.Name] {
					seen[pattern] = true
				}
			}
			return true
		})
		return true
	})
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestPagedEndpointListMatchesTheHandlers(t *testing.T) {
	paged := pagedHandlers(t)
	if len(paged) == 0 {
		t.Fatal("found no handler calling parsePaginationParams; the parse is broken, not the list")
	}
	want := pagedRoutes(t, paged)

	raw, err := os.ReadFile(pagedEndpointsFile)
	if err != nil {
		t.Fatalf("read %s: %v", pagedEndpointsFile, err)
	}
	var listed struct {
		Endpoints []string `json:"endpoints"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("decode %s: %v", pagedEndpointsFile, err)
	}

	have := map[string]bool{}
	for _, e := range listed.Endpoints {
		have[e] = true
	}
	wanted := map[string]bool{}
	for _, e := range want {
		wanted[e] = true
		if !have[e] {
			t.Errorf("%s is paged (its handler calls parsePaginationParams) but is not in web/src/test/paged-endpoints.json: add it, so the frontend guard asks every caller to page it", e)
		}
	}
	for _, e := range listed.Endpoints {
		if !wanted[e] {
			t.Errorf("web/src/test/paged-endpoints.json lists %s, which no paged handler serves: remove it", e)
		}
	}
}
