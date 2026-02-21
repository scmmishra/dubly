package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chatwoot/dubly/internal/analytics"
	"github.com/chatwoot/dubly/internal/cache"
	"github.com/chatwoot/dubly/internal/config"
	"github.com/chatwoot/dubly/internal/db"
	"github.com/chatwoot/dubly/internal/geo"
	"github.com/chatwoot/dubly/internal/handlers"
)

const testPassword = "test-secret"

func setupRouter(t *testing.T) *chi.Mux {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Password: testPassword,
		Domains:  []string{"short.io"},
	}
	linkCache, err := cache.New(100)
	if err != nil {
		t.Fatal(err)
	}
	geoReader, _ := geo.Open("")
	collector := analytics.NewCollector(database, geoReader, 1000, time.Hour)
	t.Cleanup(func() {
		collector.Shutdown()
		database.Close()
	})

	linkHandler := &handlers.LinkHandler{DB: database, Cfg: cfg, Cache: linkCache}
	redirectHandler := &handlers.RedirectHandler{DB: database, Cache: linkCache, Collector: collector}

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(handlers.AuthMiddleware(cfg.Password))
		r.Post("/links", linkHandler.Create)
		r.Get("/links", linkHandler.List)
		r.Get("/links/{id}", linkHandler.Get)
		r.Patch("/links/{id}", linkHandler.Update)
		r.Delete("/links/{id}", linkHandler.Delete)
	})
	r.NotFound(redirectHandler.ServeHTTP)
	return r
}

func authReq(method, path, body string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("X-API-Key", testPassword)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func doRequest(r *chi.Mux, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// createLink is a test helper that creates a link via the API and returns its ID.
func createLink(t *testing.T, r *chi.Mux, slug, domain, dest string) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"slug":%q,"domain":%q,"destination":%q}`, slug, domain, dest)
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("createLink: status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var link struct{ ID int64 `json:"id"` }
	if err := json.NewDecoder(rr.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}
	return link.ID
}

// --- Auth tests ---

func TestAuth_MissingAPIKey(t *testing.T) {
	r := setupRouter(t)
	req := httptest.NewRequest("GET", "/api/links", nil)
	rr := doRequest(r, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuth_WrongAPIKey(t *testing.T) {
	r := setupRouter(t)
	req := httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rr := doRequest(r, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuth_CorrectAPIKey(t *testing.T) {
	r := setupRouter(t)
	rr := doRequest(r, authReq("GET", "/api/links", ""))
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- Create tests ---

func TestCreateLink_Success(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"test","domain":"short.io","destination":"https://example.com","title":"Test"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rr.Code, rr.Body.String())
	}

	var link map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}
	if link["slug"] != "test" {
		t.Errorf("slug = %v, want %q", link["slug"], "test")
	}
	if link["domain"] != "short.io" {
		t.Errorf("domain = %v, want %q", link["domain"], "short.io")
	}
	if link["destination"] != "https://example.com" {
		t.Errorf("destination = %v, want %q", link["destination"], "https://example.com")
	}
	if link["title"] != "Test" {
		t.Errorf("title = %v, want %q", link["title"], "Test")
	}
}

func TestCreateLink_AutoGeneratesSlug(t *testing.T) {
	r := setupRouter(t)
	body := `{"domain":"short.io","destination":"https://example.com"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rr.Code, rr.Body.String())
	}

	var link map[string]any
	json.NewDecoder(rr.Body).Decode(&link)
	slug, ok := link["slug"].(string)
	if !ok || len(slug) != 6 {
		t.Errorf("slug = %q, want 6-char auto-generated slug", slug)
	}
}

func TestCreateLink_MissingDestination(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"test","domain":"short.io"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestCreateLink_MissingDomain(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"test","destination":"https://example.com"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestCreateLink_DomainNotAllowed(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"test","domain":"evil.com","destination":"https://example.com"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestCreateLink_DomainNormalizedToLowercase(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"norm","domain":"SHORT.IO","destination":"https://example.com"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rr.Code, rr.Body.String())
	}

	var link map[string]any
	json.NewDecoder(rr.Body).Decode(&link)
	if link["domain"] != "short.io" {
		t.Errorf("domain = %v, want %q (normalized to lowercase)", link["domain"], "short.io")
	}
}

func TestCreateLink_DuplicateSlug_Returns409(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"dup","domain":"short.io","destination":"https://example.com"}`
	doRequest(r, authReq("POST", "/api/links", body)) // first

	rr := doRequest(r, authReq("POST", "/api/links", body)) // duplicate
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestCreateLink_UnknownJSONField_Returns400(t *testing.T) {
	r := setupRouter(t)
	body := `{"slug":"test","domain":"short.io","destination":"https://example.com","unknown_field":"bad"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- List tests ---

func TestListLinks_DefaultPagination(t *testing.T) {
	r := setupRouter(t)
	// Create 3 links
	for i := range 3 {
		body := fmt.Sprintf(`{"slug":"list%d","domain":"short.io","destination":"https://example.com"}`, i)
		doRequest(r, authReq("POST", "/api/links", body))
	}

	rr := doRequest(r, authReq("GET", "/api/links", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["limit"].(float64)) != 25 {
		t.Errorf("limit = %v, want 25", resp["limit"])
	}
	if int(resp["total"].(float64)) != 3 {
		t.Errorf("total = %v, want 3", resp["total"])
	}
}

func TestListLinks_LimitCappedAt100(t *testing.T) {
	r := setupRouter(t)
	rr := doRequest(r, authReq("GET", "/api/links?limit=999", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if int(resp["limit"].(float64)) != 100 {
		t.Errorf("limit = %v, want 100 (capped)", resp["limit"])
	}
}

// --- Get tests ---

func TestGetLink_NotFound(t *testing.T) {
	r := setupRouter(t)
	rr := doRequest(r, authReq("GET", "/api/links/99999", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestGetLink_InvalidID(t *testing.T) {
	r := setupRouter(t)
	rr := doRequest(r, authReq("GET", "/api/links/abc", ""))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// --- Update tests ---

func TestUpdateLink_PartialUpdate(t *testing.T) {
	r := setupRouter(t)
	id := createLink(t, r, "partial", "short.io", "https://old.com")

	// Create with title and tags via direct creation, then update only destination
	// First, set title via update
	body := `{"title":"My Title","tags":"tag1"}`
	path := fmt.Sprintf("/api/links/%d", id)
	doRequest(r, authReq("PATCH", path, body))

	// Now update only destination
	body2 := `{"destination":"https://new.com"}`
	rr := doRequest(r, authReq("PATCH", path, body2))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rr.Code, rr.Body.String())
	}

	var link map[string]any
	json.NewDecoder(rr.Body).Decode(&link)
	if link["destination"] != "https://new.com" {
		t.Errorf("destination = %v, want %q", link["destination"], "https://new.com")
	}
	if link["title"] != "My Title" {
		t.Errorf("title = %v, want %q (preserved)", link["title"], "My Title")
	}
	if link["tags"] != "tag1" {
		t.Errorf("tags = %v, want %q (preserved)", link["tags"], "tag1")
	}
}

func TestUpdateLink_ClearsFieldWithEmptyString(t *testing.T) {
	r := setupRouter(t)
	// Create link then set title
	body := `{"slug":"clear","domain":"short.io","destination":"https://example.com","title":"Has Title"}`
	rr := doRequest(r, authReq("POST", "/api/links", body))
	var created map[string]any
	json.NewDecoder(rr.Body).Decode(&created)
	id := int64(created["id"].(float64))

	// Clear title with empty string
	clearBody := `{"title":""}`
	path := fmt.Sprintf("/api/links/%d", id)
	rr2 := doRequest(r, authReq("PATCH", path, clearBody))
	if rr2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr2.Code)
	}

	var updated map[string]any
	json.NewDecoder(rr2.Body).Decode(&updated)
	if updated["title"] != "" {
		t.Errorf("title = %v, want empty string", updated["title"])
	}
}

func TestUpdateLink_CacheInvalidatesOldKey(t *testing.T) {
	r := setupRouter(t)
	id := createLink(t, r, "oldslug", "short.io", "https://example.com")

	// Trigger redirect to cache the link
	req := httptest.NewRequest("GET", "/oldslug", nil)
	req.Host = "short.io"
	rr := doRequest(r, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("initial redirect: status = %d, want 302", rr.Code)
	}

	// Update slug from "oldslug" to "newslug"
	body := fmt.Sprintf(`{"slug":"newslug"}`)
	path := fmt.Sprintf("/api/links/%d", id)
	rr2 := doRequest(r, authReq("PATCH", path, body))
	if rr2.Code != http.StatusOK {
		t.Fatalf("update: status = %d, want 200, body = %s", rr2.Code, rr2.Body.String())
	}

	// Old slug should now 404 (cache invalidated)
	req2 := httptest.NewRequest("GET", "/oldslug", nil)
	req2.Host = "short.io"
	rr3 := doRequest(r, req2)
	if rr3.Code != http.StatusNotFound {
		t.Errorf("old slug after update: status = %d, want 404", rr3.Code)
	}

	// New slug should redirect
	req3 := httptest.NewRequest("GET", "/newslug", nil)
	req3.Host = "short.io"
	rr4 := doRequest(r, req3)
	if rr4.Code != http.StatusFound {
		t.Errorf("new slug: status = %d, want 302", rr4.Code)
	}
}

// --- Delete tests ---

func TestDeleteLink_Returns204(t *testing.T) {
	r := setupRouter(t)
	id := createLink(t, r, "todelete", "short.io", "https://example.com")

	path := fmt.Sprintf("/api/links/%d", id)
	rr := doRequest(r, authReq("DELETE", path, ""))
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rr.Body.String())
	}
}

func TestDeleteLink_NotFound(t *testing.T) {
	r := setupRouter(t)
	rr := doRequest(r, authReq("DELETE", "/api/links/99999", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// --- Redirect tests ---

func TestRedirect_Success(t *testing.T) {
	r := setupRouter(t)
	createLink(t, r, "go", "short.io", "https://example.com")

	req := httptest.NewRequest("GET", "/go", nil)
	req.Host = "short.io"
	rr := doRequest(r, req)
	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("Location = %q, want %q", loc, "https://example.com")
	}
}

func TestRedirect_EmptySlug_Returns404(t *testing.T) {
	r := setupRouter(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "short.io"
	rr := doRequest(r, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRedirect_UnknownSlug_Returns404(t *testing.T) {
	r := setupRouter(t)
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	req.Host = "short.io"
	rr := doRequest(r, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRedirect_InactiveLink_Returns410(t *testing.T) {
	r := setupRouter(t)
	id := createLink(t, r, "inactive", "short.io", "https://example.com")

	// Soft delete
	path := fmt.Sprintf("/api/links/%d", id)
	doRequest(r, authReq("DELETE", path, ""))

	// Redirect should return 410
	req := httptest.NewRequest("GET", "/inactive", nil)
	req.Host = "short.io"
	rr := doRequest(r, req)
	if rr.Code != http.StatusGone {
		t.Errorf("status = %d, want 410", rr.Code)
	}
}

func TestRedirect_HostNormalizedToLowercase(t *testing.T) {
	r := setupRouter(t)
	createLink(t, r, "hosttest", "short.io", "https://example.com")

	req := httptest.NewRequest("GET", "/hosttest", nil)
	req.Host = "SHORT.IO"
	rr := doRequest(r, req)
	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 (host normalization)", rr.Code)
	}
}
