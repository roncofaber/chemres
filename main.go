package main

import (
	"log"
	"net/http"

	"github.com/roncofaber/chem-resolver/internal/handlers"
	"github.com/roncofaber/chem-resolver/internal/resolver"
)

func main() {
	tmpl := handlers.MustLoadTemplates("templates")

	resolvers := []resolver.Resolver{
		resolver.NewSmilesResolver(),
		resolver.NewNameResolver(),
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/export", handlers.NewExportHandler(tmpl))

	for _, r := range resolvers {
		id := r.SystemID()
		mux.Handle("/"+id+"/resolve", handlers.NewResolveHandler(tmpl, r))
		mux.Handle("/"+id+"/batch", handlers.NewBatchHandler(tmpl, r))
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		tmpl.ExecuteTemplate(w, "index.html", map[string]any{
			"Resolvers": resolvers,
		})
	})

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
