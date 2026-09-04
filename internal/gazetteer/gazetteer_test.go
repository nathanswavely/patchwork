package gazetteer

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// build writes a throwaway index and returns a reader on it.
func build(t *testing.T, places ...Place) *Gazetteer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gaz.db")
	b, err := NewBuilder(path)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	for _, p := range places {
		if err := b.Add(p); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := b.Finish("test", 40.0379, -76.3055, 25); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	g, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// The builder and the query path fold text with the same function, and
// nothing but this test notices when they stop. A file indexed one way and
// queried another answers nothing at all, while every other test of either
// half in isolation still passes.
func TestBuilderAndQueryTokenizeAlike(t *testing.T) {
	p := Place{Name: "Lanc Workshop & Tool Library", HouseNumber: "433", Street: "Ice Avenue", City: "Lancaster", Postcode: "17602", Latitude: 40.03, Longitude: -76.30}
	g := build(t, p)

	got, ok := g.Suggest(p.Label())
	if !ok {
		t.Fatal("a place indexed from its own text was not found by its own label")
	}
	if got.Street != "Ice Avenue" {
		t.Fatalf("wrong place: %+v", got)
	}
}

// OSM writes "Avenue" and people type "Ave". Both sides canonicalize, so the
// two are the same token by the time anything is compared.
func TestAbbreviationsMatchTheirLongForm(t *testing.T) {
	g := build(t, Place{HouseNumber: "433", Street: "Ice Avenue", City: "Lancaster", Latitude: 40.03, Longitude: -76.30})

	for _, q := range []string{"433 Ice Ave", "433 Ice Avenue, Lancaster", "433 ice ave lancaster"} {
		if _, ok := g.Suggest(q); !ok {
			t.Errorf("%q found nothing", q)
		}
	}
}

// ADR 046 made locations name-first, so the name is what somebody types.
func TestNamedVenueBeatsAnAddressSharingAWord(t *testing.T) {
	g := build(t,
		Place{Name: "The Selvage", Street: "North Prince Street", City: "Lancaster", Latitude: 40.0392, Longitude: -76.3050},
		Place{HouseNumber: "12", Street: "Selvage Road", City: "Lancaster", Latitude: 40.0100, Longitude: -76.3400},
	)
	got, ok := g.Suggest("The Selvage, Lancaster")
	if !ok {
		t.Fatal("named venue not found")
	}
	if got.Name != "The Selvage" {
		t.Fatalf("matched the road, not the venue: %+v", got)
	}
}

// A housenumber is the only token that distinguishes one building on a street
// from every other, so it has to outweigh the tokens they all share.
func TestHouseNumberPicksTheRightBuilding(t *testing.T) {
	g := build(t,
		Place{HouseNumber: "433", Street: "Ice Avenue", City: "Lancaster", Latitude: 40.0300, Longitude: -76.3000},
		Place{HouseNumber: "600", Street: "Ice Avenue", City: "Lancaster", Latitude: 40.0310, Longitude: -76.3010},
	)
	got, ok := g.Suggest("600 Ice Avenue, Lancaster")
	if !ok {
		t.Fatal("nothing found")
	}
	if got.HouseNumber != "600" {
		t.Fatalf("picked %s, wanted 600", got.HouseNumber)
	}
}

// The same street name in two townships is the case where guessing would put
// a marker in the wrong town and dress it as an answer.
func TestTheSameStreetInTwoTownsSuggestsNothing(t *testing.T) {
	g := build(t,
		Place{Street: "Main Street", City: "Lititz", Latitude: 40.1573, Longitude: -76.3077},
		Place{Street: "Main Street", City: "Mountville", Latitude: 40.0384, Longitude: -76.4319},
	)
	if got, ok := g.Suggest("Main Street"); ok {
		t.Fatalf("an ambiguous street was answered with %+v", got)
	}
}

// Two entries for one doorway are not ambiguous, they are the same answer
// twice, so the tie must not suppress a perfectly good suggestion.
func TestATieAtTheSameSpotStillAnswers(t *testing.T) {
	g := build(t,
		Place{Street: "Ice Avenue", City: "Lancaster", Latitude: 40.03000, Longitude: -76.30000},
		Place{Street: "Ice Avenue", City: "Lancaster", Latitude: 40.03050, Longitude: -76.30020},
	)
	if _, ok := g.Suggest("Ice Avenue, Lancaster"); !ok {
		t.Fatal("two entries for one place suppressed the suggestion")
	}
}

// CONTEXT.md blesses this exact string as a valid address. It resolves to
// nothing, and that has to be quiet rather than an error.
func TestProseAddressIsNotAnError(t *testing.T) {
	g := build(t, Place{HouseNumber: "40", Street: "West King Street", City: "Lancaster", Latitude: 40.0379, Longitude: -76.3055})
	if _, ok := g.Suggest("above the record shop"); ok {
		t.Fatal("prose matched something")
	}
}

// One token is a city or a street shared by hundreds of rows. Nothing it
// selects is a placement anybody meant.
func TestASingleTokenSuggestsNothing(t *testing.T) {
	g := build(t, Place{Street: "Ice Avenue", City: "Lancaster", Latitude: 40.03, Longitude: -76.30})
	for _, q := range []string{"Lancaster", "  ", ""} {
		if _, ok := g.Suggest(q); ok {
			t.Errorf("%q produced a suggestion", q)
		}
	}
}

// An instance without the file is the common case, not a broken one.
func TestNoGazetteerAnswersNothing(t *testing.T) {
	var g *Gazetteer
	if _, ok := g.Suggest("433 Ice Avenue"); ok {
		t.Fatal("a nil gazetteer answered")
	}
	if g.Count() != 0 {
		t.Fatal("a nil gazetteer counted places")
	}
	if err := g.Close(); err != nil {
		t.Fatalf("closing a nil gazetteer: %v", err)
	}
	if _, err := Open(""); err != ErrNotConfigured {
		t.Fatalf("empty path should be ErrNotConfigured, got %v", err)
	}
}

// A configured path that is not a gazetteer is the admin's mistake and has to
// say so, rather than serving nothing and looking like an empty region.
func TestOpenRejectsAFileThatIsNotAGazetteer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated (x TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("a foreign database opened as a gazetteer")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "absent.db")); err == nil {
		t.Fatal("a missing file opened as a gazetteer")
	}
}

// Name-first, the contract docs/adr/046 set for assembled location strings.
func TestLabelIsNameFirst(t *testing.T) {
	p := Place{Name: "The Selvage", HouseNumber: "433", Street: "Ice Avenue", City: "Lancaster"}
	if got, want := p.Label(), "The Selvage, 433 Ice Avenue, Lancaster"; got != want {
		t.Fatalf("Label() = %q, want %q", got, want)
	}
	bare := Place{HouseNumber: "433", Street: "Ice Avenue"}
	if got, want := bare.Label(), "433 Ice Avenue"; got != want {
		t.Fatalf("Label() = %q, want %q", got, want)
	}
}

func TestDistanceMetres(t *testing.T) {
	// Roughly 13 km apart; the assertion is loose because the point is the
	// order of magnitude, not the geodesy.
	d := DistanceMetres(40.0379, -76.3055, 40.1573, -76.3077)
	if d < 12000 || d > 14500 {
		t.Fatalf("distance %.0f m is not in the expected range", d)
	}
	if DistanceMetres(40, -76, 40, -76) != 0 {
		t.Fatal("a point is not zero metres from itself")
	}
}
