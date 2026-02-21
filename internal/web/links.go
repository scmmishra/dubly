package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/chatwoot/dubly/internal/models"
	"github.com/chatwoot/dubly/internal/slug"
)

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
		"destination": r.FormValue("destination"),
		"domain":      r.FormValue("domain"),
		"slug":        r.FormValue("slug"),
		"title":       r.FormValue("title"),
		"tags":        r.FormValue("tags"),
		"notes":       r.FormValue("notes"),
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

	link := &models.Link{
		Slug:        slugVal,
		Domain:      domain,
		Destination: values["destination"],
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
	http.Redirect(w, r, "/admin/links", http.StatusFound)
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

	data := LinkFormData{
		PageData: h.pageData(w, r),
		Link:     link,
		Domains:  h.cfg.Domains,
		Errors:   map[string]string{},
		Values: map[string]string{
			"destination": link.Destination,
			"domain":      link.Domain,
			"slug":        link.Slug,
			"title":       link.Title,
			"tags":        link.Tags,
			"notes":       link.Notes,
		},
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
		"destination": r.FormValue("destination"),
		"domain":      r.FormValue("domain"),
		"slug":        r.FormValue("slug"),
		"title":       r.FormValue("title"),
		"tags":        r.FormValue("tags"),
		"notes":       r.FormValue("notes"),
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

	existing.Slug = values["slug"]
	existing.Domain = domain
	existing.Destination = values["destination"]
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
