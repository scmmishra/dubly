package geo

import (
	"net"

	"github.com/oschwald/maxminddb-golang"
)

type Result struct {
	Country   string
	City      string
	Region    string
	Latitude  float64
	Longitude float64
}

type Reader struct {
	db *maxminddb.Reader
}

// Open opens a MaxMind .mmdb file. Returns nil Reader (no-op) if path is empty.
func Open(path string) (*Reader, error) {
	if path == "" {
		return &Reader{}, nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &Reader{db: db}, nil
}

func (r *Reader) Close() {
	if r != nil && r.db != nil {
		r.db.Close()
	}
}

// Lookup resolves an IP to geo data. Returns empty Result if reader has no db.
func (r *Reader) Lookup(ipStr string) Result {
	if r == nil || r.db == nil {
		return Result{}
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Result{}
	}

	var record struct {
		Country struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		Location struct {
			Latitude  float64 `maxminddb:"latitude"`
			Longitude float64 `maxminddb:"longitude"`
		} `maxminddb:"location"`
	}

	if err := r.db.Lookup(ip, &record); err != nil {
		return Result{}
	}

	res := Result{
		Country:   record.Country.ISOCode,
		City:      record.City.Names["en"],
		Latitude:  record.Location.Latitude,
		Longitude: record.Location.Longitude,
	}
	if len(record.Subdivisions) > 0 {
		res.Region = record.Subdivisions[0].Names["en"]
	}
	return res
}
