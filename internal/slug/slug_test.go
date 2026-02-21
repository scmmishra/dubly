package slug

import (
	"regexp"
	"testing"
)

func TestGenerate_Length(t *testing.T) {
	for i := 0; i < 100; i++ {
		s, err := Generate()
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if len(s) != 6 {
			t.Fatalf("iteration %d: len = %d, want 6 (slug=%q)", i, len(s), s)
		}
	}
}

func TestGenerate_Charset(t *testing.T) {
	re := regexp.MustCompile(`^[0-9A-Za-z]{6}$`)
	for i := 0; i < 100; i++ {
		s, err := Generate()
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if !re.MatchString(s) {
			t.Fatalf("iteration %d: slug %q does not match [0-9A-Za-z]{6}", i, s)
		}
	}
}

func TestGenerate_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		s, err := Generate()
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if seen[s] {
			t.Fatalf("duplicate slug %q at iteration %d", s, i)
		}
		seen[s] = true
	}
}
