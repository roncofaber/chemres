package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

type SuggestHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewSuggestHandler(tmpl *template.Template, r resolver.Resolver) *SuggestHandler {
	return &SuggestHandler{tmpl: tmpl, resolver: r}
}

func (h *SuggestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("input"))
	if len(query) < 2 {
		return
	}
	suggestions, err := h.resolver.Suggest(query)
	if err != nil || len(suggestions) == 0 {
		return
	}
	h.tmpl.ExecuteTemplate(w, "suggest.html", suggestions)
}
