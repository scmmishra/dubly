package web

import (
	"crypto/subtle"
	"database/sql"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/scmmishra/dubly/internal/cache"
	"github.com/scmmishra/dubly/internal/config"
)

type AdminHandler struct {
	db        *sql.DB
	cfg       *config.Config
	cache     *cache.LinkCache
	templates *TemplateRegistry
	appName   string
	dns       *dnsCache
}

func NewAdminHandler(db *sql.DB, cfg *config.Config, linkCache *cache.LinkCache) (*AdminHandler, error) {
	tmpl, err := NewTemplateRegistry()
	if err != nil {
		return nil, err
	}

	return &AdminHandler{
		db:        db,
		cfg:       cfg,
		cache:     linkCache,
		templates: tmpl,
		appName:   cfg.AppName,
		dns:       newDNSCache(),
	}, nil
}

func (h *AdminHandler) RegisterRoutes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		// Static files (no auth)
		staticSub, _ := fs.Sub(staticFS, "static")
		r.Handle("/static/*", http.StripPrefix("/admin/static/", http.FileServer(http.FS(staticSub))))

		// Public routes
		r.Get("/login", h.LoginPage)
		r.Post("/login", h.LoginSubmit)

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(SessionMiddleware(h.cfg.Password))

			r.Post("/logout", h.Logout)
			r.Get("/", h.LinkList)
			r.Get("/links/new", h.LinkNewPage)
			r.Post("/links", h.LinkCreate)
			r.Get("/links/{id}/edit", h.LinkEditPage)
			r.Post("/links/{id}", h.LinkUpdate)
			r.Delete("/links/{id}", h.LinkDelete)
			r.Get("/links/{id}/analytics", h.LinkAnalytics)
			r.Get("/links/{id}/qr", h.LinkQRCode)
			r.Get("/domains", h.DomainsPage)
			r.Post("/domains/refresh", h.DomainsRefresh)
		})
	})
}

type PageData struct {
	Flash   *Flash
	AppName string
}

type LoginData struct {
	Error   string
	AppName string
}

func (h *AdminHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to dashboard
	if verifySession(r, h.cfg.Password) {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	h.templates.Render(w, "templates/login.html", LoginData{AppName: h.appName})
}

func (h *AdminHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")

	if subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.Password)) != 1 {
		h.templates.Render(w, "templates/login.html", LoginData{Error: "Invalid password", AppName: h.appName})
		return
	}

	createSession(w, h.cfg.Password)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	destroySession(w)
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

func (h *AdminHandler) pageData(w http.ResponseWriter, r *http.Request) PageData {
	return PageData{
		Flash:   getFlash(w, r),
		AppName: h.appName,
	}
}
