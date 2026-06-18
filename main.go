package main

import (
	"log"
	"net/http"

	"github.com/roncofaber/chem-resolver/internal/handlers"
	"github.com/roncofaber/chem-resolver/internal/resolver"
)

func main() {
	tmpl := handlers.MustLoadTemplates("templates")
	r    := resolver.NewAutoResolver()

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/export",  handlers.NewExportHandler(tmpl))
	mux.Handle("/resolve", handlers.NewResolveHandler(tmpl, r))
	mux.Handle("/batch",   handlers.NewBatchHandler(tmpl, r))
	mux.Handle("/suggest", handlers.NewSuggestHandler(tmpl, r))

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		tmpl.ExecuteTemplate(w, "index.html", nil)
	})

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
