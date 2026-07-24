package handler

import (
	"encoding/json"
	"fmt"
	"testing"
)

// decode is a helper that round-trips a JSON literal through the same
// decoding the handlers use, so test inputs match real request shapes.
func decode(t *testing.T, raw string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("bad test input %q: %v", raw, err)
	}
	return v
}

func TestNormalizeAppearanceValid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"full", `{"palette":"anthem","block":"pinwheel","rotation":90}`, `{"palette":"anthem","block":"pinwheel","rotation":90}`},
		{"palette only (migrated shape)", `{"palette":"anthem"}`, `{"palette":"anthem"}`},
		{"block only", `{"block":"logCabin"}`, `{"block":"logCabin"}`},
		{"explicit rotation zero", `{"block":"railFence","rotation":0}`, `{"block":"railFence","rotation":0}`},
		{"unknown-but-valid slug stored opaquely", `{"palette":"chainsGrass"}`, `{"palette":"chainsGrass"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeAppearance(decode(t, c.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil || *got != c.want {
				t.Fatalf("got %v, want %s", got, c.want)
			}
		})
	}
}

func TestNormalizeAppearanceDraftValid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"minimal draft",
			`{"block":{"grid":1}}`,
			`{"block":{"grid":1}}`,
		},
		{
			"hourglass draft with colors and bundle",
			`{"block":{"grid":1,"seams":[[0,0,4,4],[4,0,0,4]],"colors":{"0,0":[0,1,2,1]}},"bundle":["#C02624","#D89E13","#261922"]}`,
			`{"block":{"grid":1,"seams":[[0,0,4,4],[4,0,0,4]],"colors":{"0,0":[0,1,2,1]}},"bundle":["#C02624","#D89E13","#261922"]}`,
		},
		{
			"cross-grid seam on a coarse grid (midpoint anchors)",
			`{"block":{"grid":10,"seams":[[0,0,40,40],[20,0,40,20]]}}`,
			`{"block":{"grid":10,"seams":[[0,0,40,40],[20,0,40,20]]}}`,
		},
		{
			"quarter anchor legal on a fine grid",
			`{"block":{"grid":5,"seams":[[1,0,20,20]]}}`,
			`{"block":{"grid":5,"seams":[[1,0,20,20]]}}`,
		},
		{
			"bundle recolors a curated block (one color system)",
			`{"block":"pinwheel","bundle":["#039BE6","#EC341C","#0a0a0a"]}`,
			`{"block":"pinwheel","bundle":["#039BE6","#EC341C","#0a0a0a"]}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeAppearance(decode(t, c.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil || *got != c.want {
				t.Fatalf("got %v, want %s", got, c.want)
			}
		})
	}
}

func TestNormalizeAppearanceDraftInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"grid zero", `{"block":{"grid":0}}`},
		{"grid too large", `{"block":{"grid":11}}`},
		{"grid missing", `{"block":{"seams":[[0,0,4,4]]}}`},
		{"grid non-integer", `{"block":{"grid":2.5}}`},
		{"unknown draft field", `{"block":{"grid":2,"stitches":[]}}`},
		{"seam budget exceeded", `{"block":{"grid":2,"seams":[` + repeatSeams(25) + `]}}`},
		{"seam wrong arity", `{"block":{"grid":2,"seams":[[0,0,8]]}}`},
		{"seam out of bounds", `{"block":{"grid":2,"seams":[[0,0,9,8]]}}`},
		{"seam interior anchor", `{"block":{"grid":2,"seams":[[1,1,8,8]]}}`},
		{"seam degenerate", `{"block":{"grid":2,"seams":[[4,0,4,0]]}}`},
		{"quarter anchor on coarse grid", `{"block":{"grid":6,"seams":[[1,0,24,24]]}}`},
		{"non-integer coordinate", `{"block":{"grid":2,"seams":[[0.5,0,8,8]]}}`},
		{"colors bad key", `{"block":{"grid":2,"colors":{"a,b":[0]}}}`},
		{"colors cell outside grid", `{"block":{"grid":2,"colors":{"2,0":[0]}}}`},
		{"colors slot out of range", `{"block":{"grid":2,"colors":{"0,0":[6]}}}`},
		{"colors slot negative", `{"block":{"grid":2,"colors":{"0,0":[-1]}}}`},
		{"colors not arrays", `{"block":{"grid":2,"colors":{"0,0":1}}}`},
		{"block wrong type", `{"block":42}`},
		{"bundle empty", `{"bundle":[]}`},
		{"bundle too many", `{"bundle":["#111111","#222222","#333333","#444444","#555555","#666666","#777777"]}`},
		{"bundle not hex", `{"bundle":["red"]}`},
		{"bundle wrong type", `{"bundle":"#111111"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := normalizeAppearance(decode(t, c.in)); err == nil {
				t.Errorf("%s: expected error, got none", c.in)
			}
		})
	}
}

// repeatSeams builds n copies of a valid seam literal, comma-joined.
func repeatSeams(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += ","
		}
		s += "[0,0,8,8]"
	}
	return s
}

// TestMaxedDraftFitsSnapshotCap: a worst-case draft must stay well under
// the remote-follow snapshot cap (8KB) that carries appearance across
// quilts (docs/adr/024, docs/adr/029).
func TestMaxedDraftFitsSnapshotCap(t *testing.T) {
	seams := "["
	for i := 0; i < 24; i++ {
		if i > 0 {
			seams += ","
		}
		seams += `[0,0,40,40]`
	}
	seams += "]"
	colors := "{"
	for r := 0; r < 10; r++ {
		for c := 0; c < 10; c++ {
			if r+c > 0 {
				colors += ","
			}
			colors += fmt.Sprintf(`"%d,%d":[0,1,2,3,4,5]`, r, c)
		}
	}
	colors += "}"
	raw := `{"block":{"grid":10,"seams":` + seams + `,"colors":` + colors + `},"bundle":["#111111","#222222","#333333","#444444","#555555","#666666"],"rotation":270,"icon":"star"}`
	got, err := normalizeAppearance(decode(t, raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected canonical JSON, got nil")
	}
	if len(*got) > 4096 {
		t.Fatalf("maxed draft is %d bytes; must stay well under the 8KB snapshot cap", len(*got))
	}
}

func TestNormalizeAppearanceUnset(t *testing.T) {
	for _, raw := range []string{`null`, `{}`} {
		got, err := normalizeAppearance(decode(t, raw))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", raw, err)
		}
		if got != nil {
			t.Fatalf("%s: want nil (SQL NULL), got %q", raw, *got)
		}
	}
}

func TestNormalizeAppearanceInvalid(t *testing.T) {
	cases := []string{
		`"anthem"`,                          // not an object
		`42`,                                // not an object
		`["anthem"]`,                        // not an object
		`{"palette":""}`,                    // empty slug
		`{"palette":"has spaces"}`,          // bad charset
		`{"palette":"x!@#"}`,                // bad charset
		`{"block":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, // 33 chars
		`{"rotation":45}`,                   // not a quarter turn
		`{"rotation":-90}`,                  // negative
		`{"rotation":90.5}`,                 // non-integer
		`{"rotation":"90"}`,                 // string rotation
		`{"palette":"anthem","extra":true}`, // unknown field
		`{"theme":"anthem"}`,                // legacy field name rejected
	}
	for _, raw := range cases {
		if _, err := normalizeAppearance(decode(t, raw)); err == nil {
			t.Errorf("%s: expected error, got none", raw)
		}
	}
}
