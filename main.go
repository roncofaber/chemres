package main

import (
	"log"
	"net/http"
	"os"

	"github.com/roncofaber/chemres/internal/handlers"
	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

func main() {
	tmpl  := handlers.MustLoadTemplates("templates")
	r     := resolver.NewAutoResolver()
	store := jobs.NewStore()

	mux := http.NewServeMux()
	mux.Handle("/static/",      http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/export",       handlers.NewExportHandler(tmpl))
	mux.Handle("/resolve",      handlers.NewResolveHandler(tmpl, r))
	mux.Handle("/batch/start",  handlers.NewBatchStartHandler(tmpl, r, store))
	mux.Handle("/batch/stream", handlers.NewBatchStreamHandler(tmpl, r, store))
	mux.Handle("/suggest",      handlers.NewSuggestHandler(tmpl, r))

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		if err := tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
			log.Printf("template error: %v", err)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
