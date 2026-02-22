package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type TemplateRegistry struct {
	cache map[string]*template.Template
	mu    sync.RWMutex
}

func NewTemplateRegistry() (*TemplateRegistry, error) {
	funcMap := templateFuncMap()

	layout, err := template.New("layout.html").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html")
	if err != nil {
		return nil, err
	}

	tr := &TemplateRegistry{
		cache: make(map[string]*template.Template),
	}

	// Page templates that use the layout
	pages := []string{
		"templates/link_new.html",
		"templates/link_edit.html",
		"templates/link_analytics.html",
		"templates/domains.html",
	}

	for _, page := range pages {
		t, err := template.Must(layout.Clone()).ParseFS(templateFS, page)
		if err != nil {
			return nil, err
		}
		tr.cache[page] = t
	}

	// Links page needs the cards partial too
	linksT, err := template.Must(layout.Clone()).ParseFS(templateFS, "templates/links.html", "templates/links_cards.html")
	if err != nil {
		return nil, err
	}
	tr.cache["templates/links.html"] = linksT

	// Cards partial (standalone, for HTMX requests)
	partial, err := template.New("links_cards.html").Funcs(funcMap).ParseFS(templateFS, "templates/links_cards.html")
	if err != nil {
		return nil, err
	}
	tr.cache["templates/links_cards.html"] = partial

	// Login (standalone, no layout)
	login, err := template.New("login.html").Funcs(funcMap).ParseFS(templateFS, "templates/login.html")
	if err != nil {
		return nil, err
	}
	tr.cache["templates/login.html"] = login

	return tr, nil
}

func (tr *TemplateRegistry) Render(w http.ResponseWriter, name string, data any) {
	tr.mu.RLock()
	t, ok := tr.cache[name]
	tr.mu.RUnlock()

	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (tr *TemplateRegistry) RenderPartial(w io.Writer, name, block string, data any) error {
	tr.mu.RLock()
	t, ok := tr.cache[name]
	tr.mu.RUnlock()

	if !ok {
		return fmt.Errorf("template not found: %s", name)
	}

	return t.ExecuteTemplate(w, block, data)
}
