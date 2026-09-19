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

// Found against a real index built from the September 2026 Pennsylvania
// extract: "150 N Prince St, Lancaster" suggested the East King Street
// Garage, a different street entirely. 150 North Prince Street does not
// exist, and the East King row shared {150, street, lancaster} — three hits
// plus the housenumber weight plus the city — while every genuine North
// Prince row shared four tokens and no number. The generic word "Street" did
// the matching and the housenumber paid for it.
func TestAHouseNumberOnTheWrongStreetDoesNotWin(t *testing.T) {
	g := build(t,
		Place{Name: "East King Street Garage", HouseNumber: "150", Street: "East King Street", City: "Lancaster", Latitude: 40.03826, Longitude: -76.30232},
		Place{Name: "Prince Street Parking Garage", HouseNumber: "111", Street: "North Prince Street", City: "Lancaster", Latitude: 40.04000, Longitude: -76.30800},
	)
	got, ok := g.Suggest("150 N Prince St, Lancaster")
	if !ok {
		t.Fatal("nothing found")
	}
	if got.Street != "North Prince Street" {
		t.Fatalf("suggested a different street: %+v", got)
	}
}

// And when the right street cannot say which building, saying nothing is the
// answer — not the confident wrong street the housenumber used to buy.
func TestAnAbsentHouseNumberFallsBackToSilenceNotAnotherStreet(t *testing.T) {
	g := build(t,
		Place{Name: "East King Street Garage", HouseNumber: "150", Street: "East King Street", City: "Lancaster", Latitude: 40.03826, Longitude: -76.30232},
		Place{HouseNumber: "111", Street: "North Prince Street", City: "Lancaster", Latitude: 40.04000, Longitude: -76.30800},
		Place{HouseNumber: "301", Street: "North Prince Street", City: "Lancaster", Latitude: 40.04400, Longitude: -76.30830},
	)
	if got, ok := g.Suggest("150 N Prince St, Lancaster"); ok {
		t.Fatalf("a number that exists on no named-street row was answered with %+v", got)
	}
}

// The gate cannot key on the whole street name: people drop the direction far
// more often than they invent one, so "40 King St" has to still find West
// King Street — and has to prefer it over the same number on another street.
func TestADroppedDirectionStillMatchesTheStreet(t *testing.T) {
	g := build(t,
		Place{HouseNumber: "40", Street: "West King Street", City: "Lancaster", Latitude: 40.03790, Longitude: -76.30550},
		Place{HouseNumber: "40", Street: "North Queen Street", City: "Lancaster", Latitude: 40.04200, Longitude: -76.30300},
	)
	got, ok := g.Suggest("40 King St, Lancaster")
	if !ok {
		t.Fatal("nothing found")
	}
	if got.Street != "West King Street" {
		t.Fatalf("picked %+v", got)
	}
}

// A street whose entire name is a direction has to keep it, or it has no name
// left to be confirmed by and its housenumber never scores.
func TestAStreetNamedOnlyByItsDirectionStillScores(t *testing.T) {
	g := build(t,
		Place{HouseNumber: "22", Street: "North Street", City: "Lititz", Latitude: 40.15730, Longitude: -76.30770},
		Place{HouseNumber: "22", Street: "Main Street", City: "Lititz", Latitude: 40.15300, Longitude: -76.31400},
	)
	got, ok := g.Suggest("22 North Street, Lititz")
	if !ok {
		t.Fatal("nothing found")
	}
	if got.Street != "North Street" {
		t.Fatalf("picked %+v", got)
	}
}

func TestStreetCore(t *testing.T) {
	cases := []struct {
		street string
		want   []string
	}{
		{"East King Street", []string{"king"}},
		{"North Prince Street", []string{"prince"}},
		{"Old Philadelphia Pike", []string{"old", "philadelphia"}},
		{"North Street", []string{"north"}}, // nothing else to keep
		{"Street", nil},                     // no name to confirm
		{"", nil},                           // no street at all
	}
	for _, c := range cases {
		got := streetCore(c.street)
		if len(got) != len(c.want) {
			t.Errorf("streetCore(%q) = %v, want %v", c.street, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("streetCore(%q) = %v, want %v", c.street, got, c.want)
				break
			}
		}
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

// Found by building an index at county scale: a unique address was being
// suppressed as ambiguous. "Millersville Road, Millersville" tokenizes to one
// `millersville`, so the same number on the same street in the next town over
// scored identically and the pair cancelled out — naming the city bought
// nothing at the one moment it was the only thing that could help.
func TestTheCityNamedBreaksATieBetweenTowns(t *testing.T) {
	g := build(t,
		Place{HouseNumber: "2130", Street: "Millersville Road", City: "Millersville", Latitude: 39.9975, Longitude: -76.3550},
		Place{HouseNumber: "2130", Street: "Millersville Road", City: "Lancaster", Latitude: 40.2490, Longitude: -76.1000},
	)
	got, ok := g.Suggest("2130 Millersville Road, Millersville")
	if !ok {
		t.Fatal("a unique address was suppressed as ambiguous")
	}
	if got.City != "Millersville" {
		t.Fatalf("picked the wrong town: %+v", got)
	}

	// And the other way round, so this is the city deciding rather than the
	// first row winning.
	got, ok = g.Suggest("2130 Millersville Road, Lancaster")
	if !ok || got.City != "Lancaster" {
		t.Fatalf("naming the other city did not pick it: ok=%v %+v", ok, got)
	}
}

// A city nobody typed must not score. Otherwise every row in the county gets
// a bonus and the tie-break stops meaning anything.
func TestAnUnmentionedCityScoresNothing(t *testing.T) {
	g := build(t,
		Place{HouseNumber: "12", Street: "Ice Avenue", City: "Lititz", Latitude: 40.1573, Longitude: -76.3077},
		Place{HouseNumber: "12", Street: "Ice Avenue", City: "Mountville", Latitude: 40.0384, Longitude: -76.4319},
	)
	if got, ok := g.Suggest("12 Ice Avenue"); ok {
		t.Fatalf("naming no city still picked a town: %+v", got)
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

// One-word venue names are the norm on an arts instance (877 of a real
// Lancaster index's 29,812 places have one), and the guard above made every
// one of them unreachable. A word that is the whole of a place's name is the
// one single token that names a doorway rather than a town.
func TestAOneWordVenueNameIsFound(t *testing.T) {
	g := build(t,
		Place{Name: "Tellus360", HouseNumber: "24", Street: "East King Street", City: "Lancaster", Latitude: 40.0378, Longitude: -76.3041},
		Place{HouseNumber: "26", Street: "East King Street", City: "Lancaster", Latitude: 40.0378, Longitude: -76.3040},
		Place{Name: "Tellus Wellness Studio", HouseNumber: "9", Street: "North Prince Street", City: "Lancaster", Latitude: 40.0400, Longitude: -76.3060},
	)
	got, ok := g.Suggest("Tellus360")
	if !ok {
		t.Fatal("a one-word venue name found nothing")
	}
	if got.Name != "Tellus360" {
		t.Fatalf("matched something else: %+v", got)
	}
}

// The narrow path must stay narrow. A word that labels a town or a road is
// exactly the word the guard exists for, and a place happening to be named
// after one does not buy it an answer.
func TestAOneWordCityOrStreetStillSuggestsNothing(t *testing.T) {
	g := build(t,
		// The city node an extract carries alongside its addresses.
		Place{Name: "Lancaster", Latitude: 40.0379, Longitude: -76.3055},
		Place{HouseNumber: "433", Street: "Ice Avenue", City: "Lancaster", Latitude: 40.0300, Longitude: -76.3000},
		// A street drawn as a way is indexed under its own name.
		Place{Name: "Prince Street", Latitude: 40.0390, Longitude: -76.3060},
		Place{HouseNumber: "9", Street: "North Prince Street", City: "Lancaster", Latitude: 40.0400, Longitude: -76.3061},
	)
	for _, q := range []string{"Lancaster", "Prince", "Ice"} {
		if got, ok := g.Suggest(q); ok {
			t.Errorf("%q was answered with %+v", q, got)
		}
	}
}

// A chain names a dozen buildings and none of them is the one meant, so the
// same distance rule that refuses "Main Street in three townships" has to
// refuse this too.
func TestAOneWordNameOnSeveralBuildingsSuggestsNothing(t *testing.T) {
	g := build(t,
		Place{Name: "Sheetz", HouseNumber: "1500", Street: "Manheim Pike", City: "Lancaster", Latitude: 40.0700, Longitude: -76.3100},
		Place{Name: "Sheetz", HouseNumber: "2", Street: "South Broad Street", City: "Lititz", Latitude: 40.1573, Longitude: -76.3077},
	)
	if got, ok := g.Suggest("Sheetz"); ok {
		t.Fatalf("a name on two far-apart buildings was answered with %+v", got)
	}

	// But two rows for one doorway, the node and the building way OSM often
	// carries for the same venue, are the same answer twice, not a tie.
	g = build(t,
		Place{Name: "Zoetropolis", HouseNumber: "112", Street: "North Water Street", City: "Lancaster", Latitude: 40.03900, Longitude: -76.31000},
		Place{Name: "Zoetropolis", Street: "North Water Street", City: "Lancaster", Latitude: 40.03910, Longitude: -76.31005},
	)
	if _, ok := g.Suggest("Zoetropolis"); !ok {
		t.Fatal("two entries for one venue suppressed the suggestion")
	}
}

// A bare housenumber is not an address. The weight that makes "433" decisive
// inside "433 Ice Avenue" must not make it an answer on its own.
func TestABareHouseNumberSuggestsNothing(t *testing.T) {
	g := build(t, Place{HouseNumber: "433", Street: "Ice Avenue", City: "Lancaster", Latitude: 40.03, Longitude: -76.30})
	if got, ok := g.Suggest("433"); ok {
		t.Fatalf("a bare housenumber was answered with %+v", got)
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
