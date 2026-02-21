package cache

import (
	"testing"

	"github.com/scmmishra/dubly/internal/models"
)

func TestCache_SetAndGet(t *testing.T) {
	c, err := New(10)
	if err != nil {
		t.Fatal(err)
	}

	link := &models.Link{ID: 1, Slug: "abc", Domain: "d.co", Destination: "https://example.com"}
	c.Set("d.co", "abc", link)

	got, found := c.Get("d.co", "abc")
	if !found {
		t.Fatal("expected cache hit")
	}
	if got.ID != 1 || got.Destination != "https://example.com" {
		t.Errorf("got %+v, want link with ID=1", got)
	}
}

func TestCache_GetMiss(t *testing.T) {
	c, err := New(10)
	if err != nil {
		t.Fatal(err)
	}

	_, found := c.Get("d.co", "nonexistent")
	if found {
		t.Error("expected cache miss")
	}
}

func TestCache_Invalidate(t *testing.T) {
	c, err := New(10)
	if err != nil {
		t.Fatal(err)
	}

	link := &models.Link{ID: 1, Slug: "abc", Domain: "d.co"}
	c.Set("d.co", "abc", link)
	c.Invalidate("d.co", "abc")

	_, found := c.Get("d.co", "abc")
	if found {
		t.Error("expected cache miss after invalidate")
	}
}

func TestCache_EvictsLRU(t *testing.T) {
	c, err := New(2)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("d.co", "a", &models.Link{ID: 1})
	c.Set("d.co", "b", &models.Link{ID: 2})
	// Access "a" to make "b" the LRU
	c.Get("d.co", "a")
	// Insert "c" — should evict "b" (LRU)
	c.Set("d.co", "c", &models.Link{ID: 3})

	if _, found := c.Get("d.co", "b"); found {
		t.Error("expected 'b' to be evicted")
	}
	if _, found := c.Get("d.co", "a"); !found {
		t.Error("expected 'a' to still be cached")
	}
	if _, found := c.Get("d.co", "c"); !found {
		t.Error("expected 'c' to be cached")
	}
}
