package handlers

import (
	"database/sql"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/scmmishra/dubly/internal/analytics"
	"github.com/scmmishra/dubly/internal/cache"
	"github.com/scmmishra/dubly/internal/models"
)

type RedirectHandler struct {
	DB        *sql.DB
	Cache     *cache.LinkCache
	Collector *analytics.Collector
}

func (h *RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Strip port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)

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

	// chi's RealIP middleware already sets RemoteAddr from X-Forwarded-For/X-Real-IP
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
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
