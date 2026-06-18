package handlers

import (
	"html/template"
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
		h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Input must not be empty."},
			SystemID:       h.resolver.SystemID(),
		})
		return
	}
	res, err := h.resolver.Resolve(input)
	if err != nil {
		h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Could not reach PubChem — please try again."},
			SystemID:       h.resolver.SystemID(),
		})
		return
	}
	h.tmpl.ExecuteTemplate(w, "result.html", resultData{
		CompoundResult: res,
		SystemID:       h.resolver.SystemID(),
	})
}
