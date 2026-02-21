package models

import (
	"database/sql"
	"testing"

	"github.com/scmmishra/dubly/internal/db"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestCreateLink_SetsIDAndTimestamps(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "abc", Domain: "d.co", Destination: "https://example.com"}

	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}
	if l.ID <= 0 {
		t.Errorf("ID = %d, want > 0", l.ID)
	}
	if l.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if l.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
	if !l.IsActive {
		t.Error("IsActive = false, want true")
	}
}

func TestCreateLink_DuplicateSlugDomain(t *testing.T) {
	d := testDB(t)
	l1 := &Link{Slug: "dup", Domain: "d.co", Destination: "https://a.com"}
	if err := CreateLink(d, l1); err != nil {
		t.Fatal(err)
	}

	l2 := &Link{Slug: "dup", Domain: "d.co", Destination: "https://b.com"}
	if err := CreateLink(d, l2); err == nil {
		t.Fatal("expected UNIQUE constraint error")
	}
}

func TestCreateLink_SameSlugDifferentDomain(t *testing.T) {
	d := testDB(t)
	l1 := &Link{Slug: "same", Domain: "a.co", Destination: "https://a.com"}
	if err := CreateLink(d, l1); err != nil {
		t.Fatal(err)
	}

	l2 := &Link{Slug: "same", Domain: "b.co", Destination: "https://b.com"}
	if err := CreateLink(d, l2); err != nil {
		t.Fatalf("same slug on different domain should succeed: %v", err)
	}
}

func TestGetLinkByID_NotFound(t *testing.T) {
	d := testDB(t)
	l := &Link{ID: 99999}

	err := GetLinkByID(d, l)
	if err != sql.ErrNoRows {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestGetLinkBySlugAndDomain_Found(t *testing.T) {
	d := testDB(t)
	orig := &Link{Slug: "found", Domain: "d.co", Destination: "https://example.com", Title: "Test"}
	if err := CreateLink(d, orig); err != nil {
		t.Fatal(err)
	}

	got, err := GetLinkBySlugAndDomain(d, "found", "d.co")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != orig.ID {
		t.Errorf("ID = %d, want %d", got.ID, orig.ID)
	}
	if got.Destination != "https://example.com" {
		t.Errorf("Destination = %q, want %q", got.Destination, "https://example.com")
	}
	if got.Title != "Test" {
		t.Errorf("Title = %q, want %q", got.Title, "Test")
	}
	if got.ShortURL != "https://d.co/found" {
		t.Errorf("ShortURL = %q, want %q", got.ShortURL, "https://d.co/found")
	}
}

func TestGetLinkBySlugAndDomain_ReturnsInactiveLinks(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "deleted", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}
	if err := SoftDeleteLink(d, l.ID); err != nil {
		t.Fatal(err)
	}

	got, err := GetLinkBySlugAndDomain(d, "deleted", "d.co")
	if err != nil {
		t.Fatalf("expected inactive link to be returned, got error: %v", err)
	}
	if got.IsActive {
		t.Error("IsActive = true, want false for soft-deleted link")
	}
}

func TestGetLinkBySlugAndDomain_NotFound(t *testing.T) {
	d := testDB(t)
	_, err := GetLinkBySlugAndDomain(d, "nope", "d.co")
	if err != sql.ErrNoRows {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

func TestSlugExists_IncludesSoftDeleted(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "ghost", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}
	if err := SoftDeleteLink(d, l.ID); err != nil {
		t.Fatal(err)
	}

	exists, err := SlugExists(d, "ghost", "d.co")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected SlugExists to return true for soft-deleted link")
	}
}

func TestSlugExists_ReturnsFalseForNonexistent(t *testing.T) {
	d := testDB(t)
	exists, err := SlugExists(d, "nope", "d.co")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected SlugExists to return false")
	}
}

func TestListLinks_PaginationAndTotal(t *testing.T) {
	d := testDB(t)
	for i := range 3 {
		l := &Link{Slug: string(rune('a' + i)), Domain: "d.co", Destination: "https://example.com"}
		if err := CreateLink(d, l); err != nil {
			t.Fatal(err)
		}
	}

	links, total, err := ListLinks(d, 2, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(links) != 2 {
		t.Errorf("len(links) = %d, want 2", len(links))
	}

	// Offset past all results
	links2, total2, err := ListLinks(d, 2, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if total2 != 3 {
		t.Errorf("total = %d, want 3", total2)
	}
	if len(links2) != 0 {
		t.Errorf("len(links) = %d, want 0 for offset=3", len(links2))
	}
}

func TestListLinks_Search(t *testing.T) {
	d := testDB(t)
	links := []Link{
		{Slug: "findme", Domain: "d.co", Destination: "https://other.com"},
		{Slug: "xyz", Domain: "d.co", Destination: "https://findme.com"},
		{Slug: "abc", Domain: "d.co", Destination: "https://other.com", Title: "findme title"},
		{Slug: "def", Domain: "d.co", Destination: "https://other.com", Tags: "findme"},
		{Slug: "nope", Domain: "d.co", Destination: "https://nope.com"},
	}
	for i := range links {
		if err := CreateLink(d, &links[i]); err != nil {
			t.Fatal(err)
		}
	}

	results, total, err := ListLinks(d, 100, 0, "findme")
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if len(results) != 4 {
		t.Errorf("len = %d, want 4", len(results))
	}
}

func TestUpdateLink_Success(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "upd", Domain: "d.co", Destination: "https://old.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}
	originalUpdatedAt := l.UpdatedAt

	l.Destination = "https://new.com"
	if err := UpdateLink(d, l); err != nil {
		t.Fatal(err)
	}
	if l.Destination != "https://new.com" {
		t.Errorf("Destination = %q, want %q", l.Destination, "https://new.com")
	}
	if !l.UpdatedAt.After(originalUpdatedAt) && l.UpdatedAt != originalUpdatedAt {
		t.Error("expected UpdatedAt to change")
	}
}

func TestUpdateLink_UniqueConstraintViolation(t *testing.T) {
	d := testDB(t)
	l1 := &Link{Slug: "one", Domain: "d.co", Destination: "https://a.com"}
	l2 := &Link{Slug: "two", Domain: "d.co", Destination: "https://b.com"}
	if err := CreateLink(d, l1); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(d, l2); err != nil {
		t.Fatal(err)
	}

	l2.Slug = "one" // conflict with l1
	if err := UpdateLink(d, l2); err == nil {
		t.Fatal("expected UNIQUE constraint error")
	}
}

func TestSoftDeleteLink_SetsInactive(t *testing.T) {
	d := testDB(t)
	l := &Link{Slug: "del", Domain: "d.co", Destination: "https://example.com"}
	if err := CreateLink(d, l); err != nil {
		t.Fatal(err)
	}

	if err := SoftDeleteLink(d, l.ID); err != nil {
		t.Fatal(err)
	}

	check := &Link{ID: l.ID}
	if err := GetLinkByID(d, check); err != nil {
		t.Fatal(err)
	}
	if check.IsActive {
		t.Error("IsActive = true, want false after soft delete")
	}
}

func TestSoftDeleteLink_NonexistentID(t *testing.T) {
	d := testDB(t)
	err := SoftDeleteLink(d, 99999)
	if err != sql.ErrNoRows {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}
