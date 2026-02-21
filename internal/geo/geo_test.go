package geo

import "testing"

func TestOpen_EmptyPath_ReturnsNoOpReader(t *testing.T) {
	r, err := Open("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil Reader")
	}
}

func TestLookup_NoOpReader_ReturnsEmptyResult(t *testing.T) {
	r, _ := Open("")
	result := r.Lookup("8.8.8.8")
	if result.Country != "" || result.City != "" || result.Region != "" {
		t.Errorf("expected empty result, got %+v", result)
	}
	if result.Latitude != 0 || result.Longitude != 0 {
		t.Errorf("expected zero lat/lng, got %f/%f", result.Latitude, result.Longitude)
	}
}

func TestLookup_InvalidIP_ReturnsEmptyResult(t *testing.T) {
	r, _ := Open("")
	result := r.Lookup("not-an-ip")
	if result != (Result{}) {
		t.Errorf("expected zero Result, got %+v", result)
	}
}

func TestClose_NoOpReader_NoPanic(t *testing.T) {
	r, _ := Open("")
	r.Close() // should not panic
}
