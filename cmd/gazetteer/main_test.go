package main

import (
	"path/filepath"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/gazetteer"
)

// buildFrom runs the two passes over a fixture and writes an index, which is
// what main does either side of its flag handling.
func buildFrom(t *testing.T, in string, lat, lon, radiusKM float64) *gazetteer.Gazetteer {
	t.Helper()
	b := &builder{centreLat: lat, centreLon: lon, radiusM: radiusKM * 1000, coords: map[int64][2]float64{}}
	if err := b.scan(in, b.node); err != nil {
		t.Fatalf("node pass: %v", err)
	}
	if err := b.scan(in, b.way); err != nil {
		t.Fatalf("way pass: %v", err)
	}
	out := filepath.Join(t.TempDir(), "gaz.db")
	w, err := gazetteer.NewBuilder(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(append([]gazetteer.Place{}, b.nodePlaces...), b.wayPlaces...) {
		if err := w.Add(p); err != nil {
			w.Abort()
			t.Fatal(err)
		}
	}
	if err := w.Finish(in, lat, lon, radiusKM); err != nil {
		t.Fatal(err)
	}
	g, err := gazetteer.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// The whole pipeline, from extract to answered query. Everything below tests
// one seam of it; this asserts the seams line up.
func TestExtractBecomesAnAnsweredQuery(t *testing.T) {
	g := buildFrom(t, "testdata/mini.osm", 40.0379, -76.3055, 25)

	got, ok := g.Suggest("433 Ice Ave, Lancaster")
	if !ok {
		t.Fatal("an address in the extract was not found")
	}
	if got.HouseNumber != "433" || got.Street != "Ice Avenue" {
		t.Fatalf("wrong place: %+v", got)
	}
}

// A building carries its address on the way, not on the nodes that draw it,
// so an index built only from nodes misses most buildings.
func TestAWayCarryingAnAddressIsIndexed(t *testing.T) {
	g := buildFrom(t, "testdata/mini.osm", 40.0379, -76.3055, 25)

	got, ok := g.Suggest("Gallery Row, Lancaster")
	if !ok {
		t.Fatal("a named building drawn as a way was not indexed")
	}
	if got.Name != "Gallery Row" {
		t.Fatalf("wrong place: %+v", got)
	}
}

// The radius is the instance's own declared reach. A state extract is mostly
// somewhere else, and keeping all of it would defeat the point of cropping.
func TestPlacesOutsideTheRadiusAreDropped(t *testing.T) {
	g := buildFrom(t, "testdata/mini.osm", 40.0379, -76.3055, 25)
	if _, ok := g.Suggest("1 Far Away Road, Erie"); ok {
		t.Fatal("a place 380 km outside the radius was indexed")
	}
	// Widen the radius and the same row arrives, which shows the drop was
	// the crop rather than a parsing failure.
	wide := buildFrom(t, "testdata/mini.osm", 40.0379, -76.3055, 600)
	if _, ok := wide.Suggest("1 Far Away Road, Erie"); !ok {
		t.Fatal("widening the radius did not pick the place up")
	}
}

// An extract is mostly geometry: trees, benches, untagged corners. They have
// coordinates and nothing anybody would type.
func TestUntypeableThingsAreNotPlaces(t *testing.T) {
	if _, ok := place(&osmElement{Tags: []struct {
		K string `xml:"k,attr"`
		V string `xml:"v,attr"`
	}{{K: "natural", V: "tree"}}}, 40, -76); ok {
		t.Fatal("a tree was indexed as a place")
	}
	if _, ok := place(&osmElement{}, 40, -76); ok {
		t.Fatal("an untagged node was indexed as a place")
	}
}
