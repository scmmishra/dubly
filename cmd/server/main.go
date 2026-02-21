package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/chatwoot/dubly/internal/analytics"
	"github.com/chatwoot/dubly/internal/cache"
	"github.com/chatwoot/dubly/internal/config"
	"github.com/chatwoot/dubly/internal/db"
	"github.com/chatwoot/dubly/internal/geo"
	"github.com/chatwoot/dubly/internal/handlers"
	"github.com/chatwoot/dubly/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	geoReader, err := geo.Open(cfg.GeoIPPath)
	if err != nil {
		log.Printf("geo: %v (geo lookups disabled)", err)
		geoReader, _ = geo.Open("")
	}
	defer geoReader.Close()

	linkCache, err := cache.New(cfg.CacheSize)
	if err != nil {
		log.Fatalf("cache: %v", err)
	}

	collector := analytics.NewCollector(database, geoReader, cfg.BufferSize, cfg.FlushInterval)

	linkHandler := &handlers.LinkHandler{
		DB:    database,
		Cfg:   cfg,
		Cache: linkCache,
	}

	redirectHandler := &handlers.RedirectHandler{
		DB:        database,
		Cache:     linkCache,
		Collector: collector,
	}

	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// API routes (authenticated)
	r.Route("/api", func(r chi.Router) {
		r.Use(handlers.AuthMiddleware(cfg.Password))
		r.Post("/links", linkHandler.Create)
		r.Get("/links", linkHandler.List)
		r.Get("/links/{id}", linkHandler.Get)
		r.Patch("/links/{id}", linkHandler.Update)
		r.Delete("/links/{id}", linkHandler.Delete)
	})

	// Admin UI
	adminHandler, err := web.NewAdminHandler(database, cfg, linkCache)
	if err != nil {
		log.Fatalf("admin: %v", err)
	}
	adminHandler.RegisterRoutes(r)

	// All other routes → redirect handler
	r.NotFound(redirectHandler.ServeHTTP)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("dubly listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown: %v", err)
	}

	collector.Shutdown()
	log.Println("goodbye")
}
