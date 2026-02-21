package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/chatwoot/dubly/internal/analytics"
	"github.com/chatwoot/dubly/internal/cache"
	"github.com/chatwoot/dubly/internal/models"
)

type RedirectHandler struct {
	DB        *sql.DB
	Cache     *cache.LinkCache
	Collector *analytics.Collector
}

func (h *RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	slug := strings.TrimPrefix(r.URL.Path, "/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	// Check cache first
	link, found := h.Cache.Get(host, slug)
	if !found {
		var err error
		link, err = models.GetLinkBySlugAndDomain(h.DB, slug, host)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h.Cache.Set(host, slug, link)
	}

	if !link.IsActive {
		w.WriteHeader(http.StatusGone)
		w.Write([]byte("This link is no longer active."))
		return
	}

	// Extract client IP
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	} else {
		ip = r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}
	}

	h.Collector.Push(analytics.RawClick{
		LinkID:    link.ID,
		ClickedAt: time.Now().UTC(),
		IP:        ip,
		UserAgent: r.UserAgent(),
		Referer:   r.Referer(),
	})

	http.Redirect(w, r, link.Destination, http.StatusFound)
}
