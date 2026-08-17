package handler

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Every starter must be a draft an admin could have drawn: the same
// validator a patch's drafted appearance goes through has to accept it
// (docs/adr/029), or the drafter would refuse to reopen it.
func TestStartersAreValidDrafts(t *testing.T) {
	for _, s := range starters {
		raw, err := json.Marshal(s.Block)
		if err != nil {
			t.Fatalf("%s: marshal: %v", s.Key, err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s: unmarshal: %v", s.Key, err)
		}
		if _, err := normalizeDraftBlock(decoded); err != nil {
			t.Errorf("%s is not a valid draft: %v", s.Key, err)
		}
		if len(s.Block.Colors) == 0 {
			t.Errorf("%s has no colored pieces — it would render as one flat square", s.Key)
		}
	}
}

// The pieces of a cell tile it exactly: no gaps, no overlaps. This is the
// property the whole renderer rests on, and it holds cell by cell because
// a seam clipped to a cell is a full chord.
func TestFacesTileEveryCell(t *testing.T) {
	for _, s := range starters {
		for r := 0; r < s.Block.Grid; r++ {
			for c := 0; c < s.Block.Grid; c++ {
				faces := facesForCell(s.Block.Seams, r, c)
				if len(faces) == 0 {
					t.Fatalf("%s cell %d,%d has no pieces", s.Key, r, c)
				}
				var total float64
				for _, f := range faces {
					total += polygonArea(f)
				}
				if math.Abs(total-16) > 1e-6 {
					t.Errorf("%s cell %d,%d: pieces cover %.6f of 16", s.Key, r, c, total)
				}
			}
		}
	}
}

func TestFaceCountsAndOrder(t *testing.T) {
	// One diagonal splits a cell in two; the upper-right triangle sorts
	// first (centroid y, then x).
	faces := facesForCell([][4]int{{0, 0, 4, 4}}, 0, 0)
	if len(faces) != 2 {
		t.Fatalf("diagonal: %d pieces, want 2", len(faces))
	}
	if got := polygonCentroid(faces[0]); got.X < got.Y {
		t.Errorf("first piece centroid %+v is not the upper-right triangle", got)
	}

	// Both diagonals make a quarter-square unit.
	if n := len(facesForCell([][4]int{{0, 0, 4, 4}, {4, 0, 0, 4}}, 0, 0)); n != 4 {
		t.Errorf("quarter-square: %d pieces, want 4", n)
	}

	// A seam along a cell wall splits nothing.
	if n := len(facesForCell([][4]int{{0, 0, 0, 4}}, 0, 0)); n != 1 {
		t.Errorf("wall-collinear seam: %d pieces, want 1", n)
	}

	// A seam that misses the cell leaves it whole.
	if n := len(facesForCell([][4]int{{4, 0, 8, 4}}, 0, 0)); n != 1 {
		t.Errorf("distant seam: %d pieces, want 1", n)
	}
}

// The same fixtures web/src/test/draftGeometry.test.js asserts on. Both
// suites have to keep passing for the browser's drafter and the server's
// renderer to agree about what a draft looks like (docs/adr/043).
func TestGeometryMirrorsTheFrontend(t *testing.T) {
	// A seam crossing many cells splits each locally.
	seam := [][4]int{{0, 0, 12, 12}}
	for _, tc := range []struct{ r, c, want int }{
		{0, 0, 2}, {1, 1, 2}, {2, 2, 2}, {0, 1, 1}, {2, 0, 1},
	} {
		if got := len(facesForCell(seam, tc.r, tc.c)); got != tc.want {
			t.Errorf("cell %d,%d: %d pieces, want %d", tc.r, tc.c, got, tc.want)
		}
	}

	// Quarter-square triangles are equal in area.
	for _, f := range facesForCell([][4]int{{0, 0, 4, 4}, {4, 0, 0, 4}}, 0, 0) {
		if math.Abs(polygonArea(f)-4) > 1e-9 {
			t.Errorf("quarter triangle area %.9f, want 4", polygonArea(f))
		}
	}

	// Piece identity survives the order the seams were sewn in.
	a := facesForCell([][4]int{{0, 0, 4, 4}, {4, 0, 0, 4}, {2, 0, 2, 4}}, 0, 0)
	b := facesForCell([][4]int{{2, 0, 2, 4}, {4, 0, 0, 4}, {0, 0, 4, 4}}, 0, 0)
	if len(a) != len(b) {
		t.Fatalf("reordered seams gave %d pieces, want %d", len(b), len(a))
	}
	for i := range a {
		ca, cb := polygonCentroid(a[i]), polygonCentroid(b[i])
		if math.Abs(ca.X-cb.X) > 1e-6 || math.Abs(ca.Y-cb.Y) > 1e-6 {
			t.Errorf("piece %d moved when seams were reordered: %+v vs %+v", i, ca, cb)
		}
	}
}

func TestRenderIconSVG(t *testing.T) {
	design := iconDesign{Block: startersByKey["pinwheel"], Bundle: []string{"#112233", "#445566"}}
	svg := renderIconSVG(design, "")
	if !strings.HasPrefix(svg, "<svg ") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("not an SVG document: %.60s", svg)
	}
	// All eight pieces are cut, but gathered into one path per fabric —
	// pieces of one fabric drawn separately show the ground through the
	// hairline between them (see renderIconSVG).
	if got := strings.Count(svg, "Z"); got != 8 {
		t.Errorf("pinwheel cut %d pieces, want 8", got)
	}
	if got := strings.Count(svg, "<path"); got != 2 {
		t.Errorf("pinwheel drew %d paths, want one per fabric (2)", got)
	}
	for _, want := range []string{"#112233", "#445566"} {
		if !strings.Contains(svg, want) {
			t.Errorf("bundle fabric %s never reached the SVG", want)
		}
	}

	// A slot past the end of a one-fabric bundle wraps rather than
	// leaving a piece unpainted.
	solo := renderIconSVG(iconDesign{Block: startersByKey["log-cabin"], Bundle: []string{"#abcdef"}}, "")
	if strings.Contains(solo, "fill=\"\"") {
		t.Error("a piece rendered with no fill")
	}

	// Junk fabrics never reach the document.
	dirty := renderIconSVG(iconDesign{
		Block:  startersByKey["nine-patch"],
		Bundle: []string{`" onload="alert(1)`, "#00ff00"},
	}, "")
	if strings.Contains(dirty, "onload") {
		t.Error("an unvalidated fabric was interpolated into the SVG")
	}
}

func TestNormalizeIconDesign(t *testing.T) {
	ok := map[string]interface{}{
		"block":  map[string]interface{}{"grid": 2.0, "seams": []interface{}{[]interface{}{0.0, 0.0, 8.0, 8.0}}},
		"bundle": []interface{}{"#ff0000"},
	}
	if _, err := normalizeIconDesign(ok); err != nil {
		t.Fatalf("valid design refused: %v", err)
	}

	bad := []struct {
		name string
		in   interface{}
	}{
		{"no block", map[string]interface{}{"bundle": []interface{}{"#ff0000"}}},
		{"block is a slug", map[string]interface{}{"block": "pinwheel"}},
		{"unknown field", map[string]interface{}{
			"block": map[string]interface{}{"grid": 2.0}, "rotation": 90.0,
		}},
		{"non-hex fabric", map[string]interface{}{
			"block": map[string]interface{}{"grid": 2.0}, "bundle": []interface{}{"red"},
		}},
		{"seam off the anchors", map[string]interface{}{
			"block": map[string]interface{}{
				"grid":  2.0,
				"seams": []interface{}{[]interface{}{1.0, 1.0, 3.0, 3.0}},
			},
		}},
		{"not an object", "pinwheel"},
	}
	for _, tc := range bad {
		if _, err := normalizeIconDesign(tc.in); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
}
