package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie = "dubly_session"
	sessionMaxAge = 7 * 24 * time.Hour
)

func createSession(w http.ResponseWriter, password string) {
	exp := time.Now().Add(sessionMaxAge).Unix()
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
	sig := signPayload(payload, password)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    payload + "." + sig,
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionMaxAge.Seconds()),
	})
}

func verifySession(r *http.Request, password string) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, sig := parts[0], parts[1]

	expected := signPayload(payload, password)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return false
	}

	// Decode payload and check expiry
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return false
	}

	// Simple JSON parsing for {"exp":123456}
	s := string(decoded)
	idx := strings.Index(s, `"exp":`)
	if idx == -1 {
		return false
	}
	numStr := s[idx+6:]
	numStr = strings.TrimRight(numStr, "}")
	exp, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return false
	}

	return time.Now().Unix() < exp
}

func destroySession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func SessionMiddleware(password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !verifySession(r, password) {
				http.Redirect(w, r, "/admin/login", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func signPayload(payload, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
