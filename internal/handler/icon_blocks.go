package handler

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
)

// Default quilt icons (docs/adr/014): server-generated SVG quilt blocks,
// reusing the block vocabulary from docs/adr/004 (tile appearance). They
// are embedded in the binary and trusted — user SVG uploads are refused.
//
// Each template takes two colors: background then foreground.

const iconViewBox = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96" role="img" aria-label="Quilt icon">`

var iconBlocks = map[string]string{
	// Four rotating half-square triangles.
	"pinwheel": iconViewBox +
		`<rect width="96" height="96" fill="%[1]s"/>` +
		`<path d="M8 8 L48 8 L48 48 Z" fill="%[2]s"/>` +
		`<path d="M88 8 L88 48 L48 48 Z" fill="%[2]s"/>` +
		`<path d="M88 88 L48 88 L48 48 Z" fill="%[2]s"/>` +
		`<path d="M8 88 L8 48 L48 48 Z" fill="%[2]s"/>` +
		`</svg>`,

	// Center square with four star points.
	"ohio-star": iconViewBox +
		`<rect width="96" height="96" fill="%[1]s"/>` +
		`<rect x="34" y="34" width="28" height="28" fill="%[2]s"/>` +
		`<path d="M34 34 L62 34 L48 8 Z" fill="%[2]s"/>` +
		`<path d="M62 34 L62 62 L88 48 Z" fill="%[2]s"/>` +
		`<path d="M62 62 L34 62 L48 88 Z" fill="%[2]s"/>` +
		`<path d="M34 62 L34 34 L8 48 Z" fill="%[2]s"/>` +
		`</svg>`,

	// Strips spiraling around a center hearth square.
	"log-cabin": iconViewBox +
		`<rect width="96" height="96" fill="%[1]s"/>` +
		`<rect x="8" y="8" width="72" height="14" fill="%[2]s"/>` +
		`<rect x="74" y="8" width="14" height="72" fill="%[2]s"/>` +
		`<rect x="16" y="74" width="72" height="14" fill="%[2]s"/>` +
		`<rect x="8" y="16" width="14" height="72" fill="%[2]s"/>` +
		`<rect x="38" y="38" width="20" height="20" fill="%[2]s"/>` +
		`</svg>`,

	// 3x3 checkerboard: corners and center.
	"nine-patch": iconViewBox +
		`<rect width="96" height="96" fill="%[1]s"/>` +
		`<rect x="8" y="8" width="26" height="26" fill="%[2]s"/>` +
		`<rect x="62" y="8" width="26" height="26" fill="%[2]s"/>` +
		`<rect x="35" y="35" width="26" height="26" fill="%[2]s"/>` +
		`<rect x="8" y="62" width="26" height="26" fill="%[2]s"/>` +
		`<rect x="62" y="62" width="26" height="26" fill="%[2]s"/>` +
		`</svg>`,

	// Three rows of geese flying up.
	"flying-geese": iconViewBox +
		`<rect width="96" height="96" fill="%[1]s"/>` +
		`<path d="M8 34 L88 34 L48 8 Z" fill="%[2]s"/>` +
		`<path d="M8 62 L88 62 L48 36 Z" fill="%[2]s"/>` +
		`<path d="M8 88 L88 88 L48 64 Z" fill="%[2]s"/>` +
		`</svg>`,

	// Corner triangles and edge bars around an open center.
	"churn-dash": iconViewBox +
		`<rect width="96" height="96" fill="%[1]s"/>` +
		`<path d="M8 8 L36 8 L8 36 Z" fill="%[2]s"/>` +
		`<path d="M88 8 L88 36 L60 8 Z" fill="%[2]s"/>` +
		`<path d="M88 88 L60 88 L88 60 Z" fill="%[2]s"/>` +
		`<path d="M8 88 L8 60 L36 88 Z" fill="%[2]s"/>` +
		`<rect x="36" y="8" width="24" height="12" fill="%[2]s"/>` +
		`<rect x="76" y="36" width="12" height="24" fill="%[2]s"/>` +
		`<rect x="36" y="76" width="24" height="12" fill="%[2]s"/>` +
		`<rect x="8" y="36" width="12" height="24" fill="%[2]s"/>` +
		`</svg>`,
}

// iconBlockKeys returns the available default icon keys, sorted for stable
// output (and stable hash assignment).
func iconBlockKeys() []string {
	keys := make([]string, 0, len(iconBlocks))
	for k := range iconBlocks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// hexColorRE matches the only color syntax we interpolate into SVG.
var hexColorRE = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

const (
	iconFallbackFG = "#039BE6" // brand sky
	iconBG         = "#F2EEE4" // raw cotton
)

// renderIconBlock renders a default icon SVG for key, tinted with the
// instance branding color when it is a well-formed hex color.
func renderIconBlock(key, brandColor string) (string, bool) {
	tpl, ok := iconBlocks[key]
	if !ok {
		return "", false
	}
	fg := iconFallbackFG
	if hexColorRE.MatchString(brandColor) {
		fg = brandColor
	}
	return fmt.Sprintf(tpl, iconBG, fg), true
}

// assignedIconBlock hash-assigns a default block from the instance name —
// stable but not chosen, mirroring tile appearance (docs/adr/004).
func assignedIconBlock(name string) string {
	keys := iconBlockKeys()
	h := fnv.New32a()
	h.Write([]byte(name))
	return keys[int(h.Sum32())%len(keys)]
}
