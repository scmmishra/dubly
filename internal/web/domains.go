package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type dnsCache struct {
	results   map[string][]string // domain → resolved IPs
	checkedAt time.Time
	mu        sync.RWMutex
}

func newDNSCache() *dnsCache {
	return &dnsCache{results: make(map[string][]string)}
}

func (c *dnsCache) refresh(domains []string) {
	results := make(map[string][]string, len(domains))
	for _, d := range domains {
		ips, err := net.LookupHost(d)
		if err != nil {
			results[d] = nil
		} else {
			results[d] = ips
		}
	}
	c.mu.Lock()
	c.results = results
	c.checkedAt = time.Now()
	c.mu.Unlock()
}

func (c *dnsCache) get() (map[string][]string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.results, c.checkedAt
}

func (c *dnsCache) isStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.results) == 0 || time.Since(c.checkedAt) > 12*time.Hour
}

type domainEntry struct {
	Name string
	IPs  string
}

type DomainsData struct {
	PageData
	Domains   []domainEntry
	CheckedAt time.Time
}

func (h *AdminHandler) DomainsPage(w http.ResponseWriter, r *http.Request) {
	if h.dns.isStale() {
		h.dns.refresh(h.cfg.Domains)
	}

	results, checkedAt := h.dns.get()

	entries := make([]domainEntry, 0, len(h.cfg.Domains))
	for _, d := range h.cfg.Domains {
		ips := results[d]
		entries = append(entries, domainEntry{
			Name: d,
			IPs:  strings.Join(ips, ", "),
		})
	}

	h.templates.Render(w, "templates/domains.html", DomainsData{
		PageData:  h.pageData(w, r),
		Domains:   entries,
		CheckedAt: checkedAt,
	})
}

func (h *AdminHandler) DomainsRefresh(w http.ResponseWriter, r *http.Request) {
	h.dns.refresh(h.cfg.Domains)
	setFlash(w, "success", "DNS records refreshed")
	http.Redirect(w, r, "/admin/domains", http.StatusFound)
}
