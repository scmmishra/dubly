package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/scmmishra/dubly/internal/cache"
	"github.com/scmmishra/dubly/internal/config"
	"github.com/scmmishra/dubly/internal/models"
	"github.com/scmmishra/dubly/internal/slug"
)

const maxBodySize = 1 << 20 // 1 MB

type LinkHandler struct {
	DB    *sql.DB
	Cfg   *config.Config
	Cache *cache.LinkCache
}

type createLinkRequest struct {
	Slug        string `json:"slug"`
	Domain      string `json:"domain"`
	Destination string `json:"destination"`
	Title       string `json:"title"`
	Tags        string `json:"tags"`
	Notes       string `json:"notes"`
}

type updateLinkRequest struct {
	Slug        string  `json:"slug"`
	Domain      string  `json:"domain"`
	Destination string  `json:"destination"`
	Title       *string `json:"title"`
	Tags        *string `json:"tags"`
	Notes       *string `json:"notes"`
}

type listResponse struct {
	Links  []models.Link `json:"links"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Destination == "" {
		jsonError(w, "destination is required", http.StatusBadRequest)
		return
	}
	if req.Domain == "" {
		jsonError(w, "domain is required", http.StatusBadRequest)
		return
	}
	req.Domain = strings.ToLower(req.Domain)
	if !h.Cfg.IsDomainAllowed(req.Domain) {
		jsonError(w, "domain not allowed", http.StatusBadRequest)
		return
	}

	// Generate slug if not provided, with collision retry
	if req.Slug == "" {
		for range 10 {
			candidate, err := slug.Generate()
			if err != nil {
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			exists, err := models.SlugExists(h.DB, candidate, req.Domain)
			if err != nil {
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !exists {
				req.Slug = candidate
				break
			}
		}
		if req.Slug == "" {
			jsonError(w, "failed to generate unique slug", http.StatusInternalServerError)
			return
		}
	}

	link := &models.Link{
		Slug:        req.Slug,
		Domain:      req.Domain,
		Destination: req.Destination,
		Title:       req.Title,
		Tags:        req.Tags,
		Notes:       req.Notes,
	}

	if err := models.CreateLink(h.DB, link); err != nil {
		if isConstraintError(err) {
			jsonError(w, "slug already exists for this domain", http.StatusConflict)
			return
		}
		jsonError(w, "failed to create link", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *LinkHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 25
	} else if limit > 100 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	search := r.URL.Query().Get("search")

	links, total, err := models.ListLinks(h.DB, limit, offset, search)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if links == nil {
		links = []models.Link{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listResponse{
		Links:  links,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (h *LinkHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	link := &models.Link{ID: id}
	if err := models.GetLinkByID(h.DB, link); err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(link)
}

func (h *LinkHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Get existing link to know old slug/domain for cache invalidation
	existing := &models.Link{ID: id}
	if err := models.GetLinkByID(h.DB, existing); err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req updateLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	req.Domain = strings.ToLower(req.Domain)
	if req.Domain != "" && !h.Cfg.IsDomainAllowed(req.Domain) {
		jsonError(w, "domain not allowed", http.StatusBadRequest)
		return
	}

	// Capture old key before mutation for cache invalidation
	oldDomain, oldSlug := existing.Domain, existing.Slug

	// Apply updates — only overwrite if provided
	if req.Slug != "" {
		existing.Slug = req.Slug
	}
	if req.Domain != "" {
		existing.Domain = req.Domain
	}
	if req.Destination != "" {
		existing.Destination = req.Destination
	}
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Tags != nil {
		existing.Tags = *req.Tags
	}
	if req.Notes != nil {
		existing.Notes = *req.Notes
	}

	// Invalidate old cache entry (using pre-mutation key)
	h.Cache.Invalidate(oldDomain, oldSlug)

	if err := models.UpdateLink(h.DB, existing); err != nil {
		if isConstraintError(err) {
			jsonError(w, "slug already exists for this domain", http.StatusConflict)
			return
		}
		jsonError(w, "failed to update link", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

func (h *LinkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Get the link first to invalidate cache
	link := &models.Link{ID: id}
	if err := models.GetLinkByID(h.DB, link); err == nil {
		h.Cache.Invalidate(link.Domain, link.Slug)
	}

	if err := models.SoftDeleteLink(h.DB, id); err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodySize))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func isConstraintError(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
