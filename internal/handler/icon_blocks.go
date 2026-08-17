package handler

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The quilt icon is drafted, not uploaded (docs/adr/043): it is a block
// from the same drafter patches use for their tiles (docs/adr/029),
// stored as a draft plus a bundle of fabrics and rendered to SVG here.
// Consumers — the quilt switcher, other quilts' Connected Quilts, the
// favicon — still get a plain image URL.
//
// Everything the endpoint serves is generated from validated numbers and
// hex colors. Nothing an admin types reaches the SVG, so the
// no-user-SVG rule of docs/adr/014 survives intact.

const (
	// iconCanvas is the SVG's viewBox size. It is not a pixel size — the
	// icon is vector and renders at whatever size the consumer asks for.
	iconCanvas = 96

	iconFallbackFG = "#039BE6" // brand sky, when branding.color is unset
	iconGround     = "#F2EEE4" // raw cotton, the fabric wall's default ground
)

// hexColorRE matches the only color syntax we interpolate into SVG.
var hexColorRE = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// iconDesign is what an instance stores for its icon: a drafted block and
// the fabrics it is pieced from. Rotation is deliberately absent — unlike
// a hash-assigned tile, a drafted icon is already drawn facing the way
// its author wanted.
type iconDesign struct {
	Block  *draftBlock `json:"block"`
	Bundle []string    `json:"bundle,omitempty"`
}

// --- STARTER BLOCKS ---

// A starter is a traditional block expressed as a draft. Admins used to
// pick one of these as a fixed default; now it is a place to start
// drafting, and the drafter can take it apart (docs/adr/043).
//
// Colors are materialized from the rule below at startup rather than
// written out by hand, so a starter is honest draft data — the client
// renders it with the same code it renders any other draft with.
type starterSpec struct {
	key   string
	name  string
	grid  int
	seams [][4]int
	// slot picks a piece's bundle slot from its centroid, in quarter-cell
	// units, given the cell it belongs to.
	slot func(grid, r, c int, centroid point) int
}

type starterBlock struct {
	Key   string      `json:"key"`
	Name  string      `json:"name"`
	Block *draftBlock `json:"block"`
}

// outwardness is positive for a point on the far side of a cell's center
// from the block's center — how the edge and corner units of the
// traditional blocks below know which way they face.
func outwardness(grid, r, c int, p point) float64 {
	mid := float64(grid-1) / 2
	dc, dr := float64(c)-mid, float64(r)-mid
	if dc != 0 {
		dc /= math.Abs(dc)
	}
	if dr != 0 {
		dr /= math.Abs(dr)
	}
	lx := p.X - (float64(4*c) + 2)
	ly := p.Y - (float64(4*r) + 2)
	return dc*lx + dr*ly
}

var starterSpecs = []starterSpec{
	{
		// Both block diagonals: eight wedges around the center, coloured
		// alternately, which is what makes the vanes read as spinning.
		key: "pinwheel", name: "Pinwheel", grid: 2,
		seams: [][4]int{{0, 0, 8, 8}, {8, 0, 0, 8}},
		slot: func(grid, r, c int, p point) int {
			mid := float64(2 * grid)
			ang := math.Atan2(p.Y-mid, p.X-mid)
			if ang < 0 {
				ang += 2 * math.Pi
			}
			if int(ang/(math.Pi/4))%2 == 0 {
				return 0
			}
			return 1
		},
	},
	{
		// Quarter-square triangles on the four edges, points radiating
		// out from a solid center.
		key: "ohio-star", name: "Ohio Star", grid: 3,
		seams: [][4]int{
			{4, 0, 8, 4}, {8, 0, 4, 4}, // top
			{0, 4, 4, 8}, {0, 8, 4, 4}, // left
			{8, 4, 12, 8}, {8, 8, 12, 4}, // right
			{4, 8, 8, 12}, {4, 12, 8, 8}, // bottom
		},
		slot: func(grid, r, c int, p point) int {
			if r == 1 && c == 1 {
				return 0 // center
			}
			if r != 1 && c != 1 {
				return 1 // corners are ground
			}
			lx := p.X - (float64(4*c) + 2)
			ly := p.Y - (float64(4*r) + 2)
			// The two triangles aligned with the point's direction carry
			// the star; the two beside them are ground.
			if (c == 1 && math.Abs(ly) > math.Abs(lx)) || (r == 1 && math.Abs(lx) > math.Abs(ly)) {
				return 0
			}
			return 1
		},
	},
	{
		key: "nine-patch", name: "Nine Patch", grid: 3,
		slot: func(grid, r, c int, p point) int {
			if (r+c)%2 == 0 {
				return 0
			}
			return 1
		},
	},
	{
		// Two rows of geese, each a triangle spanning the full width.
		key: "flying-geese", name: "Flying Geese", grid: 2,
		seams: [][4]int{
			{0, 4, 4, 0}, {4, 0, 8, 4},
			{0, 8, 4, 4}, {4, 4, 8, 8},
		},
		slot: func(grid, r, c int, p point) int {
			apex := float64(2 * grid)
			if p.Y >= float64(4*r)+math.Abs(p.X-apex) {
				return 0
			}
			return 1
		},
	},
	{
		// Strips spiralling around a hearth: concentric rings, alternating.
		key: "log-cabin", name: "Log Cabin", grid: 5,
		slot: func(grid, r, c int, p point) int {
			mid := (grid - 1) / 2
			ring := int(math.Max(math.Abs(float64(r-mid)), math.Abs(float64(c-mid))))
			switch {
			case ring == 0:
				return 2 // the hearth
			case ring%2 == 1:
				return 1
			default:
				return 0
			}
		},
	},
	{
		// Corner triangles and edge bars around an open center.
		key: "churn-dash", name: "Churn Dash", grid: 3,
		seams: [][4]int{
			{4, 0, 0, 4}, {8, 0, 12, 4}, // top corners
			{0, 8, 4, 12}, {12, 8, 8, 12}, // bottom corners
			{4, 2, 8, 2}, {4, 10, 8, 10}, // top and bottom bars
			{2, 4, 2, 8}, {10, 4, 10, 8}, // left and right bars
		},
		slot: func(grid, r, c int, p point) int {
			if r == 1 && c == 1 {
				return 1 // the open center
			}
			if outwardness(grid, r, c, p) > 0 {
				return 0
			}
			return 1
		},
	},
}

// starters is the materialized starter set, keyed for lookup and kept in
// spec order for display.
var starters, startersByKey = buildStarters()

func buildStarters() ([]starterBlock, map[string]*draftBlock) {
	out := make([]starterBlock, 0, len(starterSpecs))
	byKey := make(map[string]*draftBlock, len(starterSpecs))
	for _, s := range starterSpecs {
		d := &draftBlock{Grid: s.grid, Seams: s.seams, Colors: map[string][]int{}}
		for r := 0; r < s.grid; r++ {
			for c := 0; c < s.grid; c++ {
				faces := facesForCell(s.seams, r, c)
				slots := make([]int, len(faces))
				painted := false
				for i, f := range faces {
					slots[i] = s.slot(s.grid, r, c, polygonCentroid(f))
					if slots[i] != 0 {
						painted = true
					}
				}
				// An all-slot-zero cell is the default; storing it would
				// just be noise in the draft.
				if painted {
					d.Colors[fmt.Sprintf("%d,%d", r, c)] = slots
				}
			}
		}
		out = append(out, starterBlock{Key: s.key, Name: s.name, Block: d})
		byKey[s.key] = d
	}
	return out, byKey
}

// starterKeys returns the starter keys sorted, for stable hash assignment.
func starterKeys() []string {
	keys := make([]string, 0, len(startersByKey))
	for k := range startersByKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// assignedStarter hash-assigns a starter from the quilt's name — stable
// but not chosen, the same rule tiles follow when a patch hasn't picked
// an appearance (docs/adr/004).
func assignedStarter(name string) string {
	keys := starterKeys()
	h := fnv.New32a()
	h.Write([]byte(name))
	return keys[int(h.Sum32())%len(keys)]
}

// defaultBundle is what an undesigned icon is pieced from: the branding
// color against raw cotton.
func defaultBundle(brandColor string) []string {
	fg := iconFallbackFG
	if hexColorRE.MatchString(brandColor) {
		fg = brandColor
	}
	return []string{fg, iconGround}
}

// --- VALIDATION ---

// normalizeIconDesign validates a decoded icon design and returns the
// canonical JSON to store. The block goes through the same structural
// validator as a patch's drafted appearance (docs/adr/029) — one set of
// rules for one drafter.
func normalizeIconDesign(v interface{}) (string, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("icon_design must be an object or null")
	}
	d := iconDesign{}
	for k, val := range m {
		switch k {
		case "block":
			bm, ok := val.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("icon_design.block must be a drafted block object")
			}
			block, err := normalizeDraftBlock(bm)
			if err != nil {
				return "", err
			}
			d.Block = block
		case "bundle":
			arr, ok := val.([]interface{})
			if !ok || len(arr) == 0 || len(arr) > draftBundleSlots {
				return "", fmt.Errorf("icon_design.bundle must be an array of 1-%d hex colors", draftBundleSlots)
			}
			for _, e := range arr {
				s, ok := e.(string)
				if !ok || !hexColorRE.MatchString(s) {
					return "", fmt.Errorf("icon_design.bundle entries must be hex colors like #RRGGBB")
				}
				d.Bundle = append(d.Bundle, s)
			}
		default:
			return "", fmt.Errorf("unknown icon_design field %q", k)
		}
	}
	if d.Block == nil {
		return "", fmt.Errorf("icon_design.block is required")
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	if len(b) > appearanceMaxBytes {
		return "", fmt.Errorf("icon_design exceeds %d bytes", appearanceMaxBytes)
	}
	return string(b), nil
}

// parseIconDesign reads a stored design back. A design that no longer
// parses (hand-edited row, older format) is treated as unset rather than
// served broken.
func parseIconDesign(raw string) (iconDesign, bool) {
	var d iconDesign
	if err := json.Unmarshal([]byte(raw), &d); err != nil || d.Block == nil {
		return iconDesign{}, false
	}
	if d.Block.Grid < 1 || d.Block.Grid > draftMaxGrid {
		return iconDesign{}, false
	}
	return d, true
}

// --- RENDERING ---

// renderIconSVG draws a design as an SVG document. Fabrics are re-checked
// here, not only on the way in: the renderer never interpolates a string
// it has not just matched against hexColorRE.
func renderIconSVG(d iconDesign, brandColor string) string {
	block := d.Block
	if block == nil || block.Grid < 1 || block.Grid > draftMaxGrid {
		block = startersByKey[assignedStarter("")]
	}
	fabrics := make([]string, 0, len(d.Bundle))
	for _, f := range d.Bundle {
		if hexColorRE.MatchString(f) {
			fabrics = append(fabrics, f)
		}
	}
	if len(fabrics) == 0 {
		fabrics = defaultBundle(brandColor)
	}
	// A slot past the end of a shrunk bundle wraps, so every piece still
	// gets a fabric (the frontend renderer does the same).
	fabric := func(slot int) string {
		if slot < 0 {
			return fabrics[0]
		}
		if slot < len(fabrics) {
			return fabrics[slot]
		}
		return fabrics[slot%len(fabrics)]
	}

	scale := float64(iconCanvas) / float64(4*block.Grid)

	// Cut every piece, gathered by the fabric it is cut from. Pieces are
	// computed per cell, so one shape spanning cells arrives as several
	// faces that abut exactly; drawn as separate elements they each cover
	// half of the pixel their shared edge lands in and the ground shows
	// through between them as a hairline — a grid over the icon at any
	// size where cell walls miss whole pixels. Filling one path per fabric
	// rasterizes each region once, so its interior edges aren't edges.
	// The frontend renderer does the same (web/src/lib/quiltBlocks.js).
	cuts := map[string]*strings.Builder{}
	order := make([]string, 0, len(fabrics))
	for r := 0; r < block.Grid; r++ {
		for c := 0; c < block.Grid; c++ {
			slots := block.Colors[fmt.Sprintf("%d,%d", r, c)]
			for i, face := range facesForCell(block.Seams, r, c) {
				slot := 0
				if i < len(slots) {
					slot = slots[i]
				}
				f := fabric(slot)
				d, ok := cuts[f]
				if !ok {
					d = &strings.Builder{}
					cuts[f] = d
					order = append(order, f)
				}
				for j, p := range face {
					if j == 0 {
						d.WriteByte('M')
					} else {
						d.WriteByte('L')
					}
					d.WriteString(svgNum(p.X * scale))
					d.WriteByte(',')
					d.WriteString(svgNum(p.Y * scale))
				}
				d.WriteByte('Z')
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96" role="img" aria-label="Quilt icon">`)
	// Ground: the pieces tile the whole square, so this only shows if a
	// future grid ever left a gap.
	fmt.Fprintf(&b, `<rect width="96" height="96" fill="%s"/>`, fabric(len(fabrics)-1))
	// Each region is outlined in its own fabric so the ground can't show
	// through where two *different* fabrics meet either: the outline grows
	// a region by half a hairline, which closes the gap and costs the seam
	// under a pixel of position. Non-scaling, so it stays a hairline at
	// favicon size and at full width alike.
	for _, f := range order {
		fmt.Fprintf(&b, `<path d="%s" fill="%s" stroke="%s" stroke-width="1" stroke-linejoin="round" vector-effect="non-scaling-stroke"/>`,
			cuts[f].String(), f, f)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// svgNum formats a coordinate compactly: exact where the geometry is
// whole, three decimals where a seam lands off the grid.
func svgNum(f float64) string {
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(f, 'f', 3, 64), "0"), ".")
}
