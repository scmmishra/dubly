package handlers

import (
	"crypto/subtle"
	"net/http"
)

func AuthMiddleware(password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(key), []byte(password)) != 1 {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
