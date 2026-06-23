package handlers

import (
	"bufio"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

const maxBatchFileSize = 5 << 20

// batchSummary and batchData are used by both BatchStartHandler (indirectly) and BatchStreamHandler.
type batchSummary struct {
	Total    int
	Found    int
	NotFound int
	Errors   int
}

type batchData struct {
	Results     []resolver.CompoundResult
	ResultsJSON string
	Summary     batchSummary
	Error       string
	SystemID    string
}

type BatchStartHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
	store    *jobs.Store
}

func NewBatchStartHandler(tmpl *template.Template, r resolver.Resolver, store *jobs.Store) *BatchStartHandler {
	return &BatchStartHandler{tmpl: tmpl, resolver: r, store: store}
}

type startResponse struct {
	Job   string `json:"job,omitempty"`
	Total int    `json:"total,omitempty"`
	Error string `json:"error,omitempty"`
}

func (h *BatchStartHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBatchFileSize)

	var inputs []string

	if raw := strings.TrimSpace(r.FormValue("inputs")); raw != "" {
		scanner := bufio.NewScanner(strings.NewReader(raw))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				inputs = append(inputs, line)
			}
		}
		if err := scanner.Err(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(startResponse{Error: "Failed to read inputs."})
			return
		}
	} else {
		file, _, err := r.FormFile("file")
		if err != nil {
			msg := "Please provide inputs via the text area or upload a file."
			if err.Error() == "http: request body too large" {
				msg = "File too large — maximum size is 5 MB."
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(startResponse{Error: msg})
			return
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if i := strings.Index(line, "#"); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if line != "" {
				inputs = append(inputs, line)
			}
		}
		if err := scanner.Err(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(startResponse{Error: "Failed to read uploaded file."})
			return
		}
	}

	if len(inputs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(startResponse{Error: "No valid inputs found — enter one per line."})
		return
	}

	ip := realClientIP(r)
	id, job := h.store.New(len(inputs))

	go func() {
		ctx := resolver.WithClientIP(job.Ctx, ip)
		results, err := h.resolver.BatchWithProgress(ctx, inputs, func(_, _ int) { job.Incr() })
		job.Finish(results, err)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(startResponse{Job: id, Total: len(inputs)})
}
