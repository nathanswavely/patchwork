// Command gazetteer builds the local place index described in docs/adr/080.
//
// It reads an OpenStreetMap XML extract, keeps the places inside an
// instance's configured radius, and writes a SQLite file the server opens
// read-only. Run it on a machine with disk and memory to spare, then copy the
// result next to patchwork.db — the server never parses an extract, because
// the binary has to run on a Raspberry Pi 4 with 2GB of RAM.
//
//	gazetteer -in pennsylvania-latest.osm.bz2 -out data/gazetteer.db
//
// Input is OSM XML, plain or gzip- or bzip2-compressed. All three decoders
// are in the standard library, which is the whole reason this reads XML
// rather than the denser .osm.pbf: a pbf reader would put protocol buffers in
// go.mod for every fork forever, to build a file that ships separately. An
// extract you have as .pbf converts in one command:
//
//	osmium cat pennsylvania-latest.osm.pbf -o pennsylvania.osm.bz2
package main

import (
	"compress/bzip2"
	"compress/gzip"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/gazetteer"
)

func main() {
	var (
		in         = flag.String("in", "", "OpenStreetMap XML extract (.osm, .osm.gz, .osm.bz2)")
		out        = flag.String("out", "", "gazetteer file to write (default: beside the configured database)")
		configPath = flag.String("config", "patchwork.yaml", "patchwork.yaml to read the centre and radius from")
		lat        = flag.Float64("lat", 0, "centre latitude (overrides the config)")
		lon        = flag.Float64("lon", 0, "centre longitude (overrides the config)")
		radius     = flag.Float64("radius", 0, "radius in km (overrides the config)")
	)
	flag.Parse()

	if *in == "" {
		fmt.Fprintln(os.Stderr, "gazetteer: -in is required")
		flag.Usage()
		os.Exit(2)
	}

	centreLat, centreLon, radiusKM, outPath := *lat, *lon, *radius, *out
	// The instance already declares where it is and how far it reaches. Read
	// that rather than making an admin restate it, and let the flags win when
	// they are given.
	if cfg, err := config.Load(*configPath); err == nil {
		if centreLat == 0 && centreLon == 0 {
			centreLat, centreLon = cfg.Geographic.Latitude, cfg.Geographic.Longitude
		}
		if radiusKM == 0 {
			radiusKM = cfg.Geographic.Radius
		}
		if outPath == "" {
			outPath, _ = cfg.GazetteerPath()
		}
	} else if centreLat == 0 || radiusKM == 0 {
		log.Fatalf("could not read %s (%v) — pass -lat, -lon and -radius instead", *configPath, err)
	}
	if outPath == "" {
		outPath = "gazetteer.db"
	}
	if radiusKM <= 0 {
		log.Fatal("radius must be positive: set geographic.radius or pass -radius")
	}
	if centreLat == 0 && centreLon == 0 {
		log.Fatal("no centre: set geographic.latitude and longitude, or pass -lat and -lon")
	}

	log.Printf("building %s from %s", outPath, *in)
	log.Printf("keeping places within %g km of %.4f, %.4f", radiusKM, centreLat, centreLon)

	b := &builder{
		centreLat: centreLat,
		centreLon: centreLon,
		radiusM:   radiusKM * 1000,
		coords:    make(map[int64][2]float64),
	}

	// Pass one reads nodes. OSM XML orders nodes before ways, so a way's
	// position is unknowable until the nodes are in hand — and holding every
	// node in the file would be unbounded. Holding only the ones inside the
	// radius is bounded by the thing the admin configured.
	if err := b.scan(*in, b.node); err != nil {
		log.Fatalf("reading nodes: %v", err)
	}
	log.Printf("pass 1: %d addressed or named nodes, %d in-radius nodes held for ways", len(b.nodePlaces), len(b.coords))

	// Pass two reads ways: buildings and sites carry the address on the way,
	// not on the nodes that draw it.
	if err := b.scan(*in, b.way); err != nil {
		log.Fatalf("reading ways: %v", err)
	}
	log.Printf("pass 2: %d addressed or named ways", len(b.wayPlaces))

	w, err := gazetteer.NewBuilder(outPath)
	if err != nil {
		log.Fatalf("creating %s: %v", outPath, err)
	}
	for _, p := range b.nodePlaces {
		if err := w.Add(p); err != nil {
			w.Abort()
			log.Fatalf("writing: %v", err)
		}
	}
	for _, p := range b.wayPlaces {
		if err := w.Add(p); err != nil {
			w.Abort()
			log.Fatalf("writing: %v", err)
		}
	}
	if err := w.Finish(*in, centreLat, centreLon, radiusKM); err != nil {
		log.Fatalf("finishing %s: %v", outPath, err)
	}

	info, err := os.Stat(outPath)
	size := ""
	if err == nil {
		size = fmt.Sprintf(", %.1f MB", float64(info.Size())/(1024*1024))
	}
	log.Printf("wrote %s: %d places%s (%d had nothing to match on and were skipped)",
		outPath, w.Written(), size, w.Skipped())
	if w.Written() == 0 {
		log.Println("warning: the index is empty — check that the extract covers the configured centre")
	}
}

type builder struct {
	centreLat, centreLon float64
	radiusM              float64

	// coords holds in-radius node positions so pass two can place a way at
	// its first node. A building's first node is within its own footprint of
	// its centroid, and the person is about to look at the marker and
	// confirm it, so the difference does not survive to the screen.
	coords     map[int64][2]float64
	nodePlaces []gazetteer.Place
	wayPlaces  []gazetteer.Place
}

// open decodes the three encodings the standard library can read.
func open(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasSuffix(path, ".bz2"):
		return readCloser{bzip2.NewReader(f), f}, nil
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return readCloser{gz, f}, nil
	default:
		return f, nil
	}
}

type readCloser struct {
	io.Reader
	c io.Closer
}

func (r readCloser) Close() error { return r.c.Close() }

// osmElement is the shape of both a node and a way, which differ only in
// whether they carry coordinates or references.
type osmElement struct {
	ID   int64  `xml:"id,attr"`
	Lat  string `xml:"lat,attr"`
	Lon  string `xml:"lon,attr"`
	Tags []struct {
		K string `xml:"k,attr"`
		V string `xml:"v,attr"`
	} `xml:"tag"`
	Nds []struct {
		Ref int64 `xml:"ref,attr"`
	} `xml:"nd"`
}

// scan streams the file, handing each node and way to fn. Streaming matters:
// an extract does not fit in memory, and DecodeElement on one element at a
// time keeps only that element.
func (b *builder) scan(path string, fn func(name string, el *osmElement)) error {
	rc, err := open(path)
	if err != nil {
		return err
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "node", "way":
			var el osmElement
			if err := dec.DecodeElement(&el, &se); err != nil {
				return err
			}
			fn(se.Name.Local, &el)
		case "relation":
			// Relations describe multipolygons and boundaries. Nothing here
			// places a marker from one, so skip the subtree rather than
			// decode it.
			if err := dec.Skip(); err != nil {
				return err
			}
		}
	}
}

func (b *builder) node(name string, el *osmElement) {
	if name != "node" {
		return
	}
	lat, err1 := strconv.ParseFloat(el.Lat, 64)
	lon, err2 := strconv.ParseFloat(el.Lon, 64)
	if err1 != nil || err2 != nil {
		return
	}
	if gazetteer.DistanceMetres(b.centreLat, b.centreLon, lat, lon) > b.radiusM {
		return
	}
	b.coords[el.ID] = [2]float64{lat, lon}
	if p, ok := place(el, lat, lon); ok {
		b.nodePlaces = append(b.nodePlaces, p)
	}
}

func (b *builder) way(name string, el *osmElement) {
	if name != "way" || len(el.Nds) == 0 {
		return
	}
	at, ok := b.coords[el.Nds[0].Ref]
	if !ok {
		return // outside the radius, or its geometry is
	}
	if p, ok := place(el, at[0], at[1]); ok {
		b.wayPlaces = append(b.wayPlaces, p)
	}
}

// place pulls the fields worth indexing off an element's tags.
//
// A place needs either a name or a street to be findable. Everything else in
// an extract — a bench, a tree, an untagged corner of a building — has
// coordinates and nothing anybody would type, so it is not a place here.
func place(el *osmElement, lat, lon float64) (gazetteer.Place, bool) {
	var p gazetteer.Place
	p.Latitude, p.Longitude = lat, lon
	for _, t := range el.Tags {
		switch t.K {
		case "name":
			p.Name = t.V
		case "addr:housenumber":
			p.HouseNumber = t.V
		case "addr:street":
			p.Street = t.V
		case "addr:city":
			p.City = t.V
		case "addr:postcode":
			p.Postcode = t.V
		}
	}
	if p.Name == "" && p.Street == "" {
		return gazetteer.Place{}, false
	}
	return p, true
}
