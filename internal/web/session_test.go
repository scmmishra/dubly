package web

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testPassword = "test-secret"

func TestCreateAndVerifySession(t *testing.T) {
	w := httptest.NewRecorder()
	createSession(w, testPassword)

	// Extract cookie from response
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookies set")
	}

	cookie := cookies[0]
	if cookie.Name != sessionCookie {
		t.Errorf("cookie name = %q, want %q", cookie.Name, sessionCookie)
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if cookie.Path != "/admin" {
		t.Errorf("cookie path = %q, want /admin", cookie.Path)
	}

	// Verify using the cookie
	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(cookie)

	if !verifySession(req, testPassword) {
		t.Error("verifySession returned false for valid session")
	}
}

func TestVerifySession_WrongPassword(t *testing.T) {
	w := httptest.NewRecorder()
	createSession(w, testPassword)

	cookie := w.Result().Cookies()[0]
	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(cookie)

	if verifySession(req, "wrong-password") {
		t.Error("verifySession should return false for wrong password")
	}
}

func TestVerifySession_NoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin", nil)
	if verifySession(req, testPassword) {
		t.Error("verifySession should return false when no cookie")
	}
}

func TestVerifySession_TamperedPayload(t *testing.T) {
	w := httptest.NewRecorder()
	createSession(w, testPassword)

	cookie := w.Result().Cookies()[0]
	// Tamper with the value
	cookie.Value = "tampered." + cookie.Value[len(cookie.Value)-10:]
	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(cookie)

	if verifySession(req, testPassword) {
		t.Error("verifySession should return false for tampered cookie")
	}
}

func TestVerifySession_Expired(t *testing.T) {
	// Build an expired session manually
	exp := time.Now().Add(-1 * time.Hour).Unix()
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
	sig := signPayload(payload, testPassword)

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: payload + "." + sig})

	if verifySession(req, testPassword) {
		t.Error("verifySession should return false for expired session")
	}
}

func TestVerifySession_InvalidFormat(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "invalid"})

	if verifySession(req, testPassword) {
		t.Error("verifySession should return false for invalid cookie format")
	}
}

func TestDestroySession(t *testing.T) {
	w := httptest.NewRecorder()
	destroySession(w)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cookie to be set for deletion")
	}
	if cookies[0].MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", cookies[0].MaxAge)
	}
}

func TestSessionMiddleware_RedirectsWithoutSession(t *testing.T) {
	handler := SessionMiddleware(testPassword)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	loc := w.Header().Get("Location")
	if loc != "/admin/login" {
		t.Errorf("Location = %q, want /admin/login", loc)
	}
}

func TestSessionMiddleware_PassesWithValidSession(t *testing.T) {
	// Create a valid session cookie
	sw := httptest.NewRecorder()
	createSession(sw, testPassword)
	cookie := sw.Result().Cookies()[0]

	var called bool
	handler := SessionMiddleware(testPassword)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called with valid session")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
