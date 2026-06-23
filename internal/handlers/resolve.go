package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

type ResolveHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewResolveHandler(tmpl *template.Template, r resolver.Resolver) *ResolveHandler {
	return &ResolveHandler{tmpl: tmpl, resolver: r}
}

type resultData struct {
	resolver.CompoundResult
	SystemID string
}

func (h *ResolveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	input := strings.TrimSpace(r.FormValue("input"))
	if input == "" {
		if err := h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Input must not be empty."},
			SystemID:       h.resolver.SystemID(),
		}); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}
	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	res, err := h.resolver.Resolve(ctx, input)
	if err != nil {
		if err := h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Could not reach PubChem — please try again."},
			SystemID:       h.resolver.SystemID(),
		}); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}
	if err := h.tmpl.ExecuteTemplate(w, "result.html", resultData{
		CompoundResult: res,
		SystemID:       h.resolver.SystemID(),
	}); err != nil {
		log.Printf("template error: %v", err)
	}
}
