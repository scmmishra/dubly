package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlash_RoundTrip(t *testing.T) {
	// Set a flash
	w := httptest.NewRecorder()
	setFlash(w, "success", "Link created")

	// Build a request with the flash cookie
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no flash cookie set")
	}

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(cookies[0])

	// Read the flash
	w2 := httptest.NewRecorder()
	flash := getFlash(w2, req)

	if flash == nil {
		t.Fatal("flash is nil")
	}
	if flash.Type != "success" {
		t.Errorf("type = %q, want %q", flash.Type, "success")
	}
	if flash.Message != "Link created" {
		t.Errorf("message = %q, want %q", flash.Message, "Link created")
	}

	// Verify the cookie was cleared (read-once)
	clearCookies := w2.Result().Cookies()
	if len(clearCookies) == 0 {
		t.Fatal("expected clear cookie")
	}
	if clearCookies[0].MaxAge != -1 {
		t.Errorf("clear cookie MaxAge = %d, want -1", clearCookies[0].MaxAge)
	}
}

func TestFlash_NoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()

	flash := getFlash(w, req)
	if flash != nil {
		t.Errorf("expected nil flash, got %v", flash)
	}
}

func TestFlash_ErrorType(t *testing.T) {
	w := httptest.NewRecorder()
	setFlash(w, "error", "Something failed")

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(w.Result().Cookies()[0])

	w2 := httptest.NewRecorder()
	flash := getFlash(w2, req)

	if flash == nil {
		t.Fatal("flash is nil")
	}
	if flash.Type != "error" {
		t.Errorf("type = %q, want %q", flash.Type, "error")
	}
	if flash.Message != "Something failed" {
		t.Errorf("message = %q, want %q", flash.Message, "Something failed")
	}
}

func TestFlash_InvalidBase64(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(&http.Cookie{Name: flashCookie, Value: "not-valid-base64!!!"})

	w := httptest.NewRecorder()
	flash := getFlash(w, req)
	if flash != nil {
		t.Errorf("expected nil for invalid base64, got %v", flash)
	}
}
