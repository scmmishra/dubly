package web_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chatwoot/dubly/internal/analytics"
	"github.com/chatwoot/dubly/internal/cache"
	"github.com/chatwoot/dubly/internal/config"
	"github.com/chatwoot/dubly/internal/db"
	"github.com/chatwoot/dubly/internal/geo"
	"github.com/chatwoot/dubly/internal/models"
	"github.com/chatwoot/dubly/internal/web"
)

const testPassword = "test-secret"

func setupRouter(t *testing.T) (*chi.Mux, *sql.DB) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Password: testPassword,
		Domains:  []string{"short.io", "s.co"},
	}

	linkCache, err := cache.New(100)
	if err != nil {
		t.Fatal(err)
	}

	geoReader, _ := geo.Open("")
	collector := analytics.NewCollector(database, geoReader, 1000, time.Hour)

	adminHandler, err := web.NewAdminHandler(database, cfg, linkCache)
	if err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	adminHandler.RegisterRoutes(r)

	t.Cleanup(func() {
		collector.Shutdown()
		database.Close()
	})

	return r, database
}

func sessionCookie(t *testing.T, router *chi.Mux) *http.Cookie {
	t.Helper()
	form := url.Values{"password": {testPassword}}
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	for _, c := range w.Result().Cookies() {
		if c.Name == "dubly_session" {
			return c
		}
	}
	t.Fatal("no session cookie returned from login")
	return nil
}

func authGet(router *chi.Mux, cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func authPost(router *chi.Mux, cookie *http.Cookie, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// === Login Tests ===

func TestLoginPage_Renders(t *testing.T) {
	r, _ := setupRouter(t)
	req := httptest.NewRequest("GET", "/admin/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Password") {
		t.Error("login page should contain Password field")
	}
}

func TestLogin_Success(t *testing.T) {
	r, _ := setupRouter(t)
	form := url.Values{"password": {testPassword}}
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	loc := w.Header().Get("Location")
	if loc != "/admin" {
		t.Errorf("Location = %q, want /admin", loc)
	}

	// Check session cookie was set
	var hasSession bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "dubly_session" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Error("no session cookie set after successful login")
	}
}

func TestLogin_BadPassword(t *testing.T) {
	r, _ := setupRouter(t)
	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Invalid password") {
		t.Error("expected error message for bad password")
	}
}

func TestLoginPage_RedirectsIfLoggedIn(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	w := authGet(r, cookie, "/admin/login")
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d (redirect to dashboard)", w.Code, http.StatusFound)
	}
}

// === Auth Middleware Tests ===

func TestProtectedRoute_RedirectsWithoutSession(t *testing.T) {
	r, _ := setupRouter(t)
	req := httptest.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/login" {
		t.Errorf("Location = %q, want /admin/login", loc)
	}
}

// === Dashboard Tests ===

func TestDashboard_Renders(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	w := authGet(r, cookie, "/admin")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Error("dashboard should contain Dashboard heading")
	}
	if !strings.Contains(body, "Active links") {
		t.Error("dashboard should show Active links stat")
	}
}

// === Link List Tests ===

func TestLinkList_Empty(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	w := authGet(r, cookie, "/admin/links")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "No links yet") {
		t.Error("expected empty state message")
	}
}

func TestLinkList_WithLinks(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	// Create a link directly
	l := &models.Link{Slug: "abc", Domain: "short.io", Destination: "https://example.com", Title: "Example"}
	if err := models.CreateLink(database, l); err != nil {
		t.Fatal(err)
	}

	w := authGet(r, cookie, "/admin/links")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "short.io/abc") {
		t.Error("link list should contain the link URL")
	}
}

func TestLinkList_HTMXPartial(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	l := &models.Link{Slug: "htmx", Domain: "short.io", Destination: "https://example.com"}
	if err := models.CreateLink(database, l); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/admin/links", nil)
	req.AddCookie(cookie)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	// Partial should NOT contain the full layout (no <nav>)
	if strings.Contains(body, "<nav") {
		t.Error("HTMX partial should not contain nav")
	}
	if !strings.Contains(body, "short.io/htmx") {
		t.Error("partial should contain link")
	}
}

func TestLinkList_Search(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	models.CreateLink(database, &models.Link{Slug: "findme", Domain: "short.io", Destination: "https://example.com"})
	models.CreateLink(database, &models.Link{Slug: "nope", Domain: "short.io", Destination: "https://other.com"})

	w := authGet(r, cookie, "/admin/links?search=findme")
	body := w.Body.String()
	if !strings.Contains(body, "findme") {
		t.Error("search should find matching links")
	}
}

// === Create Link Tests ===

func TestLinkCreate_NewPage(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	w := authGet(r, cookie, "/admin/links/new")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "short.io") {
		t.Error("new link page should contain domain options")
	}
}

func TestLinkCreate_Success(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	form := url.Values{
		"destination": {"https://example.com/page"},
		"domain":      {"short.io"},
		"slug":        {"mylink"},
		"title":       {"My Link"},
	}

	w := authPost(r, cookie, "/admin/links", form)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}

	// Verify link was created
	link, err := models.GetLinkBySlugAndDomain(database, "mylink", "short.io")
	if err != nil {
		t.Fatalf("link not created: %v", err)
	}
	if link.Destination != "https://example.com/page" {
		t.Errorf("destination = %q, want https://example.com/page", link.Destination)
	}
}

func TestLinkCreate_AutoSlug(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	form := url.Values{
		"destination": {"https://example.com/auto"},
		"domain":      {"short.io"},
	}

	w := authPost(r, cookie, "/admin/links", form)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}

	// Verify a link was created (with auto-generated slug)
	links, total, err := models.ListLinks(database, 10, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if links[0].Slug == "" {
		t.Error("slug should be auto-generated")
	}
}

func TestLinkCreate_MissingDestination(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	form := url.Values{
		"domain": {"short.io"},
	}

	w := authPost(r, cookie, "/admin/links", form)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (re-render with error)", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Error("expected validation error for missing destination")
	}
}

func TestLinkCreate_DuplicateSlug(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	models.CreateLink(database, &models.Link{Slug: "taken", Domain: "short.io", Destination: "https://a.com"})

	form := url.Values{
		"destination": {"https://b.com"},
		"domain":      {"short.io"},
		"slug":        {"taken"},
	}

	w := authPost(r, cookie, "/admin/links", form)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "already exists") {
		t.Error("expected duplicate slug error")
	}
}

// === Edit Link Tests ===

func TestLinkEdit_Renders(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	l := &models.Link{Slug: "edit", Domain: "short.io", Destination: "https://example.com", Title: "Edit Me"}
	models.CreateLink(database, l)

	w := authGet(r, cookie, fmt.Sprintf("/admin/links/%d/edit", l.ID))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Edit Me") {
		t.Error("edit page should contain link title")
	}
	if !strings.Contains(body, "https://example.com") {
		t.Error("edit page should contain destination URL")
	}
}

func TestLinkEdit_NotFound(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	w := authGet(r, cookie, "/admin/links/99999/edit")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestLinkUpdate_Success(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	l := &models.Link{Slug: "old", Domain: "short.io", Destination: "https://old.com"}
	models.CreateLink(database, l)

	form := url.Values{
		"destination": {"https://new.com"},
		"domain":      {"short.io"},
		"slug":        {"old"},
		"title":       {"Updated"},
	}

	w := authPost(r, cookie, fmt.Sprintf("/admin/links/%d", l.ID), form)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}

	// Verify update
	updated := &models.Link{ID: l.ID}
	models.GetLinkByID(database, updated)
	if updated.Destination != "https://new.com" {
		t.Errorf("destination = %q, want https://new.com", updated.Destination)
	}
	if updated.Title != "Updated" {
		t.Errorf("title = %q, want Updated", updated.Title)
	}
}

// === Delete Link Tests ===

func TestLinkDelete_Success(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	l := &models.Link{Slug: "del", Domain: "short.io", Destination: "https://example.com"}
	models.CreateLink(database, l)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/admin/links/%d", l.ID), nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify soft delete
	check := &models.Link{ID: l.ID}
	models.GetLinkByID(database, check)
	if check.IsActive {
		t.Error("link should be inactive after delete")
	}
}

// === Analytics Tests ===

func TestLinkAnalytics_Renders(t *testing.T) {
	r, database := setupRouter(t)
	cookie := sessionCookie(t, r)

	l := &models.Link{Slug: "stats", Domain: "short.io", Destination: "https://example.com", Title: "Stats Link"}
	models.CreateLink(database, l)

	w := authGet(r, cookie, fmt.Sprintf("/admin/links/%d/analytics", l.ID))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Analytics") {
		t.Error("analytics page should contain heading")
	}
	if !strings.Contains(body, "short.io/stats") {
		t.Error("analytics page should contain short URL")
	}
}

func TestLinkAnalytics_NotFound(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	w := authGet(r, cookie, "/admin/links/99999/analytics")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// === Logout Tests ===

func TestLogout(t *testing.T) {
	r, _ := setupRouter(t)
	cookie := sessionCookie(t, r)

	form := url.Values{}
	w := authPost(r, cookie, "/admin/logout", form)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/login" {
		t.Errorf("Location = %q, want /admin/login", loc)
	}
}

// === Static Files Tests ===

func TestStaticCSS(t *testing.T) {
	r, _ := setupRouter(t)
	req := httptest.NewRequest("GET", "/admin/static/css/style.css", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "--bg:") {
		t.Error("CSS should contain design tokens")
	}
}

func TestStaticHTMX(t *testing.T) {
	r, _ := setupRouter(t)
	req := httptest.NewRequest("GET", "/admin/static/js/htmx.min.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() < 1000 {
		t.Error("HTMX JS should be substantial in size")
	}
}
