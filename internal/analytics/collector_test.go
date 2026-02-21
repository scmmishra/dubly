package analytics

import (
	"database/sql"
	"testing"
	"time"

	"github.com/scmmishra/dubly/internal/db"
	"github.com/scmmishra/dubly/internal/geo"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Insert a test link for FK constraint (id=1)
	_, err = database.Exec(`INSERT INTO links (slug, domain, destination) VALUES ('test', 'example.com', 'https://example.com')`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func clickCount(t *testing.T, database *sql.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRow("SELECT COUNT(*) FROM clicks").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCollector_FlushOnShutdown(t *testing.T) {
	database := testDB(t)
	geoReader, _ := geo.Open("")
	c := NewCollector(database, geoReader, 1000, time.Hour)

	for range 5 {
		c.Push(RawClick{LinkID: 1, ClickedAt: time.Now()})
	}
	c.Shutdown()

	if n := clickCount(t, database); n != 5 {
		t.Fatalf("count = %d, want 5", n)
	}
}

func TestCollector_PushNonBlockingWhenFull(t *testing.T) {
	database := testDB(t)
	geoReader, _ := geo.Open("")
	c := NewCollector(database, geoReader, 1, time.Hour)

	// Push 5 events — only 1 should fit, rest silently dropped, must not block
	for range 5 {
		c.Push(RawClick{LinkID: 1, ClickedAt: time.Now()})
	}
	c.Shutdown()

	if n := clickCount(t, database); n > 1 {
		t.Fatalf("count = %d, want at most 1", n)
	}
}

func TestCollector_FlushOnTicker(t *testing.T) {
	database := testDB(t)
	geoReader, _ := geo.Open("")
	c := NewCollector(database, geoReader, 1000, 50*time.Millisecond)

	for range 3 {
		c.Push(RawClick{LinkID: 1, ClickedAt: time.Now()})
	}

	// Wait for at least one tick to flush
	time.Sleep(200 * time.Millisecond)

	n := clickCount(t, database)
	if n == 0 {
		t.Fatal("expected clicks to be flushed by ticker, got 0")
	}
	c.Shutdown()
}

func TestCollector_EnrichesUserAgent(t *testing.T) {
	database := testDB(t)
	geoReader, _ := geo.Open("")
	c := NewCollector(database, geoReader, 1000, time.Hour)

	c.Push(RawClick{
		LinkID:    1,
		ClickedAt: time.Now(),
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	})
	c.Shutdown()

	var browser, deviceType string
	err := database.QueryRow("SELECT browser, device_type FROM clicks LIMIT 1").Scan(&browser, &deviceType)
	if err != nil {
		t.Fatal(err)
	}
	if browser != "Chrome" {
		t.Errorf("browser = %q, want %q", browser, "Chrome")
	}
	if deviceType != "desktop" {
		t.Errorf("device_type = %q, want %q", deviceType, "desktop")
	}
}

func TestCollector_EnrichesRefererDomain(t *testing.T) {
	database := testDB(t)
	geoReader, _ := geo.Open("")
	c := NewCollector(database, geoReader, 1000, time.Hour)

	c.Push(RawClick{
		LinkID:    1,
		ClickedAt: time.Now(),
		Referer:   "https://news.ycombinator.com/item?id=1",
	})
	c.Shutdown()

	var refererDomain string
	err := database.QueryRow("SELECT referer_domain FROM clicks LIMIT 1").Scan(&refererDomain)
	if err != nil {
		t.Fatal(err)
	}
	if refererDomain != "news.ycombinator.com" {
		t.Errorf("referer_domain = %q, want %q", refererDomain, "news.ycombinator.com")
	}
}

func TestCollector_EmptyReferer(t *testing.T) {
	database := testDB(t)
	geoReader, _ := geo.Open("")
	c := NewCollector(database, geoReader, 1000, time.Hour)

	c.Push(RawClick{
		LinkID:    1,
		ClickedAt: time.Now(),
		Referer:   "",
	})
	c.Shutdown()

	var refererDomain string
	err := database.QueryRow("SELECT referer_domain FROM clicks LIMIT 1").Scan(&refererDomain)
	if err != nil {
		t.Fatal(err)
	}
	if refererDomain != "" {
		t.Errorf("referer_domain = %q, want empty", refererDomain)
	}
}
