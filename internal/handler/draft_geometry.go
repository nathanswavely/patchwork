package handler

import (
	"math"
	"sort"
)

// Draft geometry (docs/adr/029): the pieced-block engine, in Go.
//
// This is the server-side twin of web/src/lib/draftGeometry.js — same
// algorithm, same face ordering, so a drafted block renders identically
// in the browser and in the SVG the server serves (docs/adr/042). The
// drafter itself stays in the frontend; what lives here is only what a
// renderer needs: the pieces of a cell, in a stable order.
//
// Coordinates are integers in quarter-cell units: a grid-n block spans
// 0..4n on both axes, so cell (r, c) is the square [4c, 4c+4] x
// [4r, 4r+4]. A seam clipped to a cell is a full chord (its anchors sit
// on cell walls), so every piece is a convex region of exactly one cell
// and the whole engine is incremental chord splitting.

const geomEps = 1e-9

type point struct{ X, Y float64 }

type polygon []point

func polygonArea(poly polygon) float64 {
	var a float64
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		a += p.X*q.Y - q.X*p.Y
	}
	return math.Abs(a) / 2
}

// polygonCentroid returns the area-weighted centroid, falling back to the
// vertex mean for degenerate slivers.
func polygonCentroid(poly polygon) point {
	var a, cx, cy float64
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		cross := p.X*q.Y - q.X*p.Y
		a += cross
		cx += (p.X + q.X) * cross
		cy += (p.Y + q.Y) * cross
	}
	if math.Abs(a) < geomEps {
		var sx, sy float64
		for _, p := range poly {
			sx += p.X
			sy += p.Y
		}
		n := float64(len(poly))
		return point{sx / n, sy / n}
	}
	return point{cx / (3 * a), cy / (3 * a)}
}

// clipSeamToCell clips a seam to the axis-aligned cell square
// (Liang-Barsky). ok is false when the seam misses the cell, only grazes
// it, or lies along one of its walls — a wall-collinear seam splits
// nothing.
func clipSeamToCell(seam [4]int, xmin, ymin, xmax, ymax float64) (a, b point, ok bool) {
	x1, y1 := float64(seam[0]), float64(seam[1])
	x2, y2 := float64(seam[2]), float64(seam[3])
	dx, dy := x2-x1, y2-y1
	t0, t1 := 0.0, 1.0
	edges := [4][2]float64{
		{-dx, x1 - xmin},
		{dx, xmax - x1},
		{-dy, y1 - ymin},
		{dy, ymax - y1},
	}
	for _, e := range edges {
		p, q := e[0], e[1]
		if math.Abs(p) < geomEps {
			if q < -geomEps {
				return a, b, false // parallel and outside
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return a, b, false
			}
			if t > t0 {
				t0 = t
			}
		} else {
			if t < t0 {
				return a, b, false
			}
			if t < t1 {
				t1 = t
			}
		}
	}
	if t1-t0 < geomEps {
		return a, b, false // grazes a corner or misses
	}
	a = point{x1 + t0*dx, y1 + t0*dy}
	b = point{x1 + t1*dx, y1 + t1*dy}
	if math.Abs(a.X-b.X) < geomEps && (math.Abs(a.X-xmin) < geomEps || math.Abs(a.X-xmax) < geomEps) {
		return a, b, false
	}
	if math.Abs(a.Y-b.Y) < geomEps && (math.Abs(a.Y-ymin) < geomEps || math.Abs(a.Y-ymax) < geomEps) {
		return a, b, false
	}
	return a, b, true
}

// splitConvexByLine splits a convex polygon by the line through a and b,
// returning the original polygon when the line misses its interior.
func splitConvexByLine(poly polygon, a, b point) []polygon {
	dx, dy := b.X-a.X, b.Y-a.Y
	side := make([]int, len(poly))
	var hasPos, hasNeg bool
	for i, p := range poly {
		s := dx*(p.Y-a.Y) - dy*(p.X-a.X)
		switch {
		case s > geomEps:
			side[i] = 1
			hasPos = true
		case s < -geomEps:
			side[i] = -1
			hasNeg = true
		}
	}
	if !hasPos || !hasNeg {
		return []polygon{poly}
	}

	var left, right polygon
	for i := range poly {
		j := (i + 1) % len(poly)
		if side[i] >= 0 {
			left = append(left, poly[i])
		}
		if side[i] <= 0 {
			right = append(right, poly[i])
		}
		if side[i]*side[j] < 0 {
			// The edge crosses the line: solve for the intersection.
			denom := dx*(poly[j].Y-poly[i].Y) - dy*(poly[j].X-poly[i].X)
			t := (dy*(poly[i].X-a.X) - dx*(poly[i].Y-a.Y)) / denom
			p := point{
				poly[i].X + t*(poly[j].X-poly[i].X),
				poly[i].Y + t*(poly[j].Y-poly[i].Y),
			}
			left = append(left, p)
			right = append(right, p)
		}
	}

	var out []polygon
	if len(left) >= 3 && polygonArea(left) > geomEps {
		out = append(out, left)
	}
	if len(right) >= 3 && polygonArea(right) > geomEps {
		out = append(out, right)
	}
	if len(out) == 0 {
		return []polygon{poly}
	}
	return out
}

// facesForCell returns the pieces of cell (r, c): the cell square split by
// every seam that crosses it, sorted by centroid (y, then x). That order
// is the piece's identity — the index into colors["r,c"] — and holds
// regardless of the order seams were sewn.
func facesForCell(seams [][4]int, r, c int) []polygon {
	xmin, ymin := float64(4*c), float64(4*r)
	xmax, ymax := xmin+4, ymin+4
	faces := []polygon{{
		{xmin, ymin}, {xmax, ymin}, {xmax, ymax}, {xmin, ymax},
	}}
	for _, seam := range seams {
		a, b, ok := clipSeamToCell(seam, xmin, ymin, xmax, ymax)
		if !ok {
			continue
		}
		var next []polygon
		for _, f := range faces {
			next = append(next, splitConvexByLine(f, a, b)...)
		}
		faces = next
	}
	sort.SliceStable(faces, func(i, j int) bool {
		ci, cj := polygonCentroid(faces[i]), polygonCentroid(faces[j])
		if math.Abs(ci.Y-cj.Y) > 1e-6 {
			return ci.Y < cj.Y
		}
		return ci.X < cj.X
	})
	return faces
}
