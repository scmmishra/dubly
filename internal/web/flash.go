package web

import (
	"encoding/base64"
	"net/http"
	"strings"
)

const flashCookie = "dubly_flash"

type Flash struct {
	Type    string // "success", "error"
	Message string
}

func setFlash(w http.ResponseWriter, typ, message string) {
	value := base64.RawURLEncoding.EncodeToString([]byte(typ + ":" + message))
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookie,
		Value:    value,
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   60,
	})
}

func getFlash(w http.ResponseWriter, r *http.Request) *Flash {
	cookie, err := r.Cookie(flashCookie)
	if err != nil {
		return nil
	}

	// Clear the cookie (read-once)
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookie,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   -1,
	})

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil
	}

	return &Flash{Type: parts[0], Message: parts[1]}
}
