package models

import (
	"database/sql"
	"testing"
	"time"
)

func insertTestClicks(t *testing.T, d *sql.DB, clicks []Click) {
	t.Helper()
	if err := BatchInsertClicks(d, clicks); err != nil {
		t.Fatal(err)
	}
}

func TestClickCountForLink(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "a", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	count, err := ClickCountForLink(d, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l.ID, ClickedAt: time.Now()},
		{LinkID: l.ID, ClickedAt: time.Now()},
	})

	count, err = ClickCountForLink(d, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestClickCountsForLinks(t *testing.T) {
	d := testDB(t)
	l1 := &Link{Slug: "a", Domain: "d.co", Destination: "https://example.com"}
	l2 := &Link{Slug: "b", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l1); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(d, l2); err != nil {
		t.Fatal(err)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l1.ID, ClickedAt: time.Now()},
		{LinkID: l1.ID, ClickedAt: time.Now()},
		{LinkID: l2.ID, ClickedAt: time.Now()},
	})

	counts, err := ClickCountsForLinks(d, []int64{l1.ID, l2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if counts[l1.ID] != 2 {
		t.Errorf("l1 count = %d, want 2", counts[l1.ID])
	}
	if counts[l2.ID] != 1 {
		t.Errorf("l2 count = %d, want 1", counts[l2.ID])
	}
}

func TestClickCountsForLinks_Empty(t *testing.T) {
	d := testDB(t)
	counts, err := ClickCountsForLinks(d, []int64{})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Errorf("len = %d, want 0", len(counts))
	}
}

func TestTopReferrersForLink(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "a", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l.ID, ClickedAt: time.Now(), RefererDomain: "google.com"},
		{LinkID: l.ID, ClickedAt: time.Now(), RefererDomain: "google.com"},
		{LinkID: l.ID, ClickedAt: time.Now(), RefererDomain: "twitter.com"},
		{LinkID: l.ID, ClickedAt: time.Now(), RefererDomain: ""},
	})

	refs, err := TopReferrersForLink(d, l.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("len = %d, want 2", len(refs))
	}
	if refs[0].Domain != "google.com" || refs[0].Count != 2 {
		t.Errorf("first = %v, want google.com:2", refs[0])
	}
	if refs[1].Domain != "twitter.com" || refs[1].Count != 1 {
		t.Errorf("second = %v, want twitter.com:1", refs[1])
	}
}

func TestTopCountriesForLink(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "a", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l.ID, ClickedAt: time.Now(), Country: "US"},
		{LinkID: l.ID, ClickedAt: time.Now(), Country: "US"},
		{LinkID: l.ID, ClickedAt: time.Now(), Country: "DE"},
		{LinkID: l.ID, ClickedAt: time.Now(), Country: ""},
	})

	countries, err := TopCountriesForLink(d, l.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(countries) != 2 {
		t.Fatalf("len = %d, want 2", len(countries))
	}
	if countries[0].Country != "US" || countries[0].Count != 2 {
		t.Errorf("first = %v, want US:2", countries[0])
	}
}

func TestTotalLinkCount(t *testing.T) {
	d := testDB(t)

	count, err := TotalLinkCount(d)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	l := &Link{Slug: "a", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	count, err = TotalLinkCount(d)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Soft-deleted links should not be counted
	if err := SoftDeleteLink(d, l.ID); err != nil {
		t.Fatal(err)
	}
	count, err = TotalLinkCount(d)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 after soft delete", count)
	}
}

func TestClicksAllTime(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "a", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	count, err := ClicksAllTime(d)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l.ID, ClickedAt: time.Now()},
		{LinkID: l.ID, ClickedAt: time.Now().Add(-24 * time.Hour)},
	})

	count, err = ClicksAllTime(d)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestTopLinksByClicks(t *testing.T) {
	d := testDB(t)
	l1 := &Link{Slug: "a", Domain: "d.co", Destination: "https://a.com", Title: "Link A"}
	l2 := &Link{Slug: "b", Domain: "d.co", Destination: "https://b.com", Title: "Link B"}
	if err := CreateLink(d, l1); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(d, l2); err != nil {
		t.Fatal(err)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l2.ID, ClickedAt: time.Now()},
		{LinkID: l2.ID, ClickedAt: time.Now()},
		{LinkID: l1.ID, ClickedAt: time.Now()},
	})

	top, err := TopLinksByClicks(d, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 {
		t.Fatalf("len = %d, want 2", len(top))
	}
	if top[0].Link.ID != l2.ID {
		t.Errorf("first link ID = %d, want %d", top[0].Link.ID, l2.ID)
	}
	if top[0].ClickCount != 2 {
		t.Errorf("first click count = %d, want 2", top[0].ClickCount)
	}
	if top[1].ClickCount != 1 {
		t.Errorf("second click count = %d, want 1", top[1].ClickCount)
	}
}

func TestTopLinksByClicks_ExcludesInactive(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "a", Domain: "d.co", Destination: "https://a.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}
	insertTestClicks(t, d, []Click{
		{LinkID: l.ID, ClickedAt: time.Now()},
	})

	if err := SoftDeleteLink(d, l.ID); err != nil {
		t.Fatal(err)
	}

	top, err := TopLinksByClicks(d, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 0 {
		t.Errorf("len = %d, want 0 (inactive links excluded)", len(top))
	}
}

func TestTopReferrersGlobal(t *testing.T) {
	d := testDB(t)
	l1 := &Link{Slug: "a", Domain: "d.co", Destination: "https://a.com"}
	l2 := &Link{Slug: "b", Domain: "d.co", Destination: "https://b.com"}
	if err := CreateLink(d, l1); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(d, l2); err != nil {
		t.Fatal(err)
	}

	insertTestClicks(t, d, []Click{
		{LinkID: l1.ID, ClickedAt: time.Now(), RefererDomain: "google.com"},
		{LinkID: l2.ID, ClickedAt: time.Now(), RefererDomain: "google.com"},
		{LinkID: l1.ID, ClickedAt: time.Now(), RefererDomain: "twitter.com"},
	})

	refs, err := TopReferrersGlobal(d, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("len = %d, want 2", len(refs))
	}
	if refs[0].Domain != "google.com" || refs[0].Count != 2 {
		t.Errorf("first = %v, want google.com:2", refs[0])
	}
}
