package models

import (
	"testing"
	"time"
)

func TestBatchInsertClicks_Success(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "clicks", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	clicks := []Click{
		{LinkID: l.ID, ClickedAt: time.Now()},
		{LinkID: l.ID, ClickedAt: time.Now()},
		{LinkID: l.ID, ClickedAt: time.Now()},
	}
	if err := BatchInsertClicks(d, clicks); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM clicks").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestBatchInsertClicks_EmptySlice(t *testing.T) {
	d := testDB(t)
	if err := BatchInsertClicks(d, nil); err != nil {
		t.Fatalf("unexpected error on empty batch: %v", err)
	}
}

func TestBatchInsertClicks_InvalidLinkID(t *testing.T) {
	d := testDB(t)
	clicks := []Click{
		{LinkID: 99999, ClickedAt: time.Now()},
	}
	if err := BatchInsertClicks(d, clicks); err == nil {
		t.Fatal("expected FK violation error")
	}
}

func TestBatchInsertClicks_RollsBackOnFailure(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "rollback", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	// First click valid, second has invalid FK → entire batch should roll back
	clicks := []Click{
		{LinkID: l.ID, ClickedAt: time.Now()},
		{LinkID: 99999, ClickedAt: time.Now()},
	}
	if err := BatchInsertClicks(d, clicks); err == nil {
		t.Fatal("expected error for mixed batch")
	}

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM clicks").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (rolled back)", count)
	}
}
