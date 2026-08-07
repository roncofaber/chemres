package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/roncofaber/chemres/internal/handlers"
	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

// appVersion is set at build time via -ldflags "-X main.appVersion=v1.x.x"
var appVersion = "dev"

const csp = "default-src 'self'; " +
	"script-src 'self' https://unpkg.com; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src https://fonts.gstatic.com; " +
	"img-src 'self' data: blob:; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func main() {
	tmpl  := handlers.MustLoadTemplates("templates")
	r     := resolver.NewAutoResolver()
	store := jobs.NewStore()

	api := handlers.NewAPIHandler(r)

	mux := http.NewServeMux()
	mux.Handle("/static/",      http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/export",       handlers.NewExportHandler(tmpl))
	mux.Handle("/resolve",      handlers.NewResolveHandler(tmpl, r))
	mux.Handle("/batch/start",  handlers.NewBatchStartHandler(tmpl, r, store))
	mux.Handle("/batch/stream", handlers.NewBatchStreamHandler(tmpl, r, store))
	mux.Handle("/suggest",      handlers.NewSuggestHandler(tmpl, r))
	mux.HandleFunc("/api/v1/resolve",   api.Resolve)
	mux.HandleFunc("/api/v1/batch",     api.Batch)
	mux.HandleFunc("/api/v1/suggest",   api.Suggest)
	mux.HandleFunc("/api/v1/conformer", api.Conformer)

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		theme := "light"
		if c, err := req.Cookie("chemres-theme"); err == nil {
			if c.Value == "dark" || c.Value == "light" {
				theme = c.Value
			}
		}
		if err := tmpl.ExecuteTemplate(w, "index.html", map[string]string{"Theme": theme, "Version": appVersion}); err != nil {
			log.Printf("template error: %v", err)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally unset: /batch/stream holds long-lived SSE connections.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Server starting on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
}
