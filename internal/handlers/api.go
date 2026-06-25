package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

const maxAPIBatchInputs  = 500
const maxAPIInputLength  = 5000
const maxAPIBodySize     = 1 << 20 // 1 MB

type APIHandler struct {
	resolver resolver.Resolver
}

func NewAPIHandler(r resolver.Resolver) *APIHandler {
	return &APIHandler{resolver: r}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func corsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// POST /api/v1/resolve
func (h *APIHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		apiError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Input) == "" {
		apiError(w, http.StatusBadRequest, "field 'input' is required")
		return
	}
	input := strings.TrimSpace(req.Input)
	if len(input) > maxAPIInputLength {
		apiError(w, http.StatusBadRequest, "input exceeds maximum length of 5000 characters")
		return
	}
	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	result, err := h.resolver.Resolve(ctx, input)
	if err != nil {
		apiError(w, http.StatusServiceUnavailable, "could not reach PubChem — please try again")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/v1/batch
func (h *APIHandler) Batch(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		apiError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIBodySize)
	var req struct {
		Inputs []string `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiError(w, http.StatusBadRequest, "expected JSON body with 'inputs' array (max 1 MB)")
		return
	}
	var inputs []string
	for _, s := range req.Inputs {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		if len(t) > maxAPIInputLength {
			apiError(w, http.StatusBadRequest, "one or more inputs exceed maximum length of 5000 characters")
			return
		}
		inputs = append(inputs, t)
	}
	if len(inputs) == 0 {
		apiError(w, http.StatusBadRequest, "field 'inputs' must be a non-empty array")
		return
	}
	if len(inputs) > maxAPIBatchInputs {
		apiError(w, http.StatusBadRequest, "too many inputs — maximum is 500 per request")
		return
	}
	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	results, _ := h.resolver.BatchWithProgress(ctx, inputs, func(_, _ int) {})
	writeJSON(w, http.StatusOK, results)
}

// GET /api/v1/suggest?q=...
func (h *APIHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	suggestions, err := h.resolver.Suggest(ctx, q)
	if err != nil || len(suggestions) == 0 {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}
