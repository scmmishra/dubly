package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/scmmishra/dubly/internal/models"
	"github.com/scmmishra/dubly/internal/slug"
)

var utmKeys = []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"}

// buildDestinationWithUTM strips existing UTM params from rawURL then appends
// any non-empty values from utmValues. Returns rawURL unchanged on parse error.
func buildDestinationWithUTM(rawURL string, utmValues map[string]string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	for _, k := range utmKeys {
		q.Del(k)
	}
	for _, k := range utmKeys {
		if v := utmValues[k]; v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// extractUTMValues returns a map of utm_* key → value parsed from rawURL.
// Missing params are empty strings.
func extractUTMValues(rawURL string) map[string]string {
	result := make(map[string]string, len(utmKeys))
	u, err := url.Parse(rawURL)
	if err != nil {
		for _, k := range utmKeys {
			result[k] = ""
		}
		return result
	}
	q := u.Query()
	for _, k := range utmKeys {
		result[k] = q.Get(k)
	}
	return result
}

// stripUTMParams returns rawURL with all utm_* query params removed.
func stripUTMParams(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	for _, k := range utmKeys {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

const linksPerPage = 12

type LinksData struct {
	PageData
	Links         []models.LinkWithClicks
	Search        string
	Page          int
	TotalPages    int
	Total         int
	TotalLinks    int
	ClicksToday   int
	ClicksAllTime int
	TopReferrers  []models.ReferrerCount
	TopCountries  []models.CountryCount
	TopBrowsers   []models.BrowserCount
	TopDevices    []models.DeviceCount
}

type LinkFormData struct {
	PageData
	Link    *models.Link
	Domains []string
	Errors  map[string]string
	Values  map[string]string
}

func (h *AdminHandler) LinkList(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * linksPerPage
	links, total, err := models.ListLinks(h.db, linksPerPage, offset, search)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get click counts for all links
	ids := make([]int64, len(links))
	for i, l := range links {
		ids[i] = l.ID
	}
	clickCounts, _ := models.ClickCountsForLinks(h.db, ids)

	linksWithClicks := make([]models.LinkWithClicks, len(links))
	for i, l := range links {
		linksWithClicks[i] = models.LinkWithClicks{
			Link:       l,
			ClickCount: clickCounts[l.ID],
		}
	}

	totalPages := (total + linksPerPage - 1) / linksPerPage
	if totalPages < 1 {
		totalPages = 1
	}

	// Fetch dashboard stats
	totalLinks, _ := models.TotalLinkCount(h.db)
	clicksToday, _ := models.ClicksToday(h.db)
	clicksAllTime, _ := models.ClicksAllTime(h.db)
	topReferrers, _ := models.TopReferrersGlobal(h.db, 5)
	topCountries, _ := models.TopCountriesGlobal(h.db, 5)
	topBrowsers, _ := models.TopBrowsersGlobal(h.db, 5)
	topDevices, _ := models.TopDevicesGlobal(h.db, 5)

	data := LinksData{
		PageData:      h.pageData(w, r),
		Links:         linksWithClicks,
		Search:        search,
		Page:          page,
		TotalPages:    totalPages,
		Total:         total,
		TotalLinks:    totalLinks,
		ClicksToday:   clicksToday,
		ClicksAllTime: clicksAllTime,
		TopReferrers:  topReferrers,
		TopCountries:  topCountries,
		TopBrowsers:   topBrowsers,
		TopDevices:    topDevices,
	}

	// HTMX partial rendering
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.templates.RenderPartial(w, "templates/links_cards.html", "cards", data)
		return
	}

	h.templates.Render(w, "templates/links.html", data)
}

func (h *AdminHandler) LinkNewPage(w http.ResponseWriter, r *http.Request) {
	data := LinkFormData{
		PageData: h.pageData(w, r),
		Domains:  h.cfg.Domains,
		Errors:   map[string]string{},
		Values:   map[string]string{"domain": h.cfg.Domains[0]},
	}
	h.templates.Render(w, "templates/link_new.html", data)
}

func (h *AdminHandler) LinkCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	values := map[string]string{
		"destination":  r.FormValue("destination"),
		"domain":       r.FormValue("domain"),
		"slug":         r.FormValue("slug"),
		"title":        r.FormValue("title"),
		"tags":         r.FormValue("tags"),
		"notes":        r.FormValue("notes"),
		"utm_source":   r.FormValue("utm_source"),
		"utm_medium":   r.FormValue("utm_medium"),
		"utm_campaign": r.FormValue("utm_campaign"),
		"utm_term":     r.FormValue("utm_term"),
		"utm_content":  r.FormValue("utm_content"),
	}

	errors := map[string]string{}

	if values["destination"] == "" {
		errors["destination"] = "Destination URL is required"
	}

	domain := strings.ToLower(values["domain"])
	if !h.cfg.IsDomainAllowed(domain) {
		errors["domain"] = "Domain not allowed"
	}
	values["domain"] = domain

	if len(errors) > 0 {
		data := LinkFormData{
			PageData: h.pageData(w, r),
			Domains:  h.cfg.Domains,
			Errors:   errors,
			Values:   values,
		}
		h.templates.Render(w, "templates/link_new.html", data)
		return
	}

	// Auto-generate slug if not provided
	slugVal := values["slug"]
	if slugVal == "" {
		for range 10 {
			candidate, err := slug.Generate()
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			exists, err := models.SlugExists(h.db, candidate, domain)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !exists {
				slugVal = candidate
				break
			}
		}
		if slugVal == "" {
			errors["slug"] = "Failed to generate unique slug"
			data := LinkFormData{
				PageData: h.pageData(w, r),
				Domains:  h.cfg.Domains,
				Errors:   errors,
				Values:   values,
			}
			h.templates.Render(w, "templates/link_new.html", data)
			return
		}
	}

	utmValues := map[string]string{
		"utm_source":   values["utm_source"],
		"utm_medium":   values["utm_medium"],
		"utm_campaign": values["utm_campaign"],
		"utm_term":     values["utm_term"],
		"utm_content":  values["utm_content"],
	}

	link := &models.Link{
		Slug:        slugVal,
		Domain:      domain,
		Destination: buildDestinationWithUTM(values["destination"], utmValues),
		Title:       values["title"],
		Tags:        values["tags"],
		Notes:       values["notes"],
	}

	if err := models.CreateLink(h.db, link); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			errors["slug"] = "This slug already exists for this domain"
			data := LinkFormData{
				PageData: h.pageData(w, r),
				Domains:  h.cfg.Domains,
				Errors:   errors,
				Values:   values,
			}
			h.templates.Render(w, "templates/link_new.html", data)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	setFlash(w, "success", "Link created: "+link.ShortURL)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (h *AdminHandler) LinkEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	link := &models.Link{ID: id}
	if err := models.GetLinkByID(h.db, link); err != nil {
		http.NotFound(w, r)
		return
	}

	utmVals := extractUTMValues(link.Destination)
	values := map[string]string{
		"destination":  stripUTMParams(link.Destination),
		"domain":       link.Domain,
		"slug":         link.Slug,
		"title":        link.Title,
		"tags":         link.Tags,
		"notes":        link.Notes,
		"utm_source":   utmVals["utm_source"],
		"utm_medium":   utmVals["utm_medium"],
		"utm_campaign": utmVals["utm_campaign"],
		"utm_term":     utmVals["utm_term"],
		"utm_content":  utmVals["utm_content"],
	}

	data := LinkFormData{
		PageData: h.pageData(w, r),
		Link:     link,
		Domains:  h.cfg.Domains,
		Errors:   map[string]string{},
		Values:   values,
	}
	h.templates.Render(w, "templates/link_edit.html", data)
}

func (h *AdminHandler) LinkUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	existing := &models.Link{ID: id}
	if err := models.GetLinkByID(h.db, existing); err != nil {
		http.NotFound(w, r)
		return
	}

	r.ParseForm()

	values := map[string]string{
		"destination":  r.FormValue("destination"),
		"domain":       r.FormValue("domain"),
		"slug":         r.FormValue("slug"),
		"title":        r.FormValue("title"),
		"tags":         r.FormValue("tags"),
		"notes":        r.FormValue("notes"),
		"utm_source":   r.FormValue("utm_source"),
		"utm_medium":   r.FormValue("utm_medium"),
		"utm_campaign": r.FormValue("utm_campaign"),
		"utm_term":     r.FormValue("utm_term"),
		"utm_content":  r.FormValue("utm_content"),
	}

	errors := map[string]string{}

	if values["destination"] == "" {
		errors["destination"] = "Destination URL is required"
	}
	if values["slug"] == "" {
		errors["slug"] = "Slug is required"
	}

	domain := strings.ToLower(values["domain"])
	if !h.cfg.IsDomainAllowed(domain) {
		errors["domain"] = "Domain not allowed"
	}
	values["domain"] = domain

	if len(errors) > 0 {
		data := LinkFormData{
			PageData: h.pageData(w, r),
			Link:     existing,
			Domains:  h.cfg.Domains,
			Errors:   errors,
			Values:   values,
		}
		h.templates.Render(w, "templates/link_edit.html", data)
		return
	}

	// Capture old key for cache invalidation
	oldDomain, oldSlug := existing.Domain, existing.Slug

	utmValues := map[string]string{
		"utm_source":   values["utm_source"],
		"utm_medium":   values["utm_medium"],
		"utm_campaign": values["utm_campaign"],
		"utm_term":     values["utm_term"],
		"utm_content":  values["utm_content"],
	}

	existing.Slug = values["slug"]
	existing.Domain = domain
	existing.Destination = buildDestinationWithUTM(values["destination"], utmValues)
	existing.Title = values["title"]
	existing.Tags = values["tags"]
	existing.Notes = values["notes"]

	h.cache.Invalidate(oldDomain, oldSlug)

	if err := models.UpdateLink(h.db, existing); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			errors["slug"] = "This slug already exists for this domain"
			data := LinkFormData{
				PageData: h.pageData(w, r),
				Link:     existing,
				Domains:  h.cfg.Domains,
				Errors:   errors,
				Values:   values,
			}
			h.templates.Render(w, "templates/link_edit.html", data)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	setFlash(w, "success", "Link updated")
	http.Redirect(w, r, "/admin/links/"+strconv.FormatInt(id, 10)+"/edit", http.StatusFound)
}

func (h *AdminHandler) LinkDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Get link for cache invalidation
	link := &models.Link{ID: id}
	if err := models.GetLinkByID(h.db, link); err == nil {
		h.cache.Invalidate(link.Domain, link.Slug)
	}

	if err := models.SoftDeleteLink(h.db, id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}
