package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

type BatchStreamHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
	store    *jobs.Store
}

func NewBatchStreamHandler(tmpl *template.Template, r resolver.Resolver, store *jobs.Store) *BatchStreamHandler {
	return &BatchStreamHandler{tmpl: tmpl, resolver: r, store: store}
}

func (h *BatchStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("job")
	job, ok := h.store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	lastDone := -1

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snap := job.Snapshot()

			if snap.Done != lastDone {
				lastDone = snap.Done
				fmt.Fprintf(w, "event: progress\ndata: %d/%d\n\n", snap.Done, snap.Total)
				flusher.Flush()
			}

			if !snap.Finished {
				continue
			}

			if snap.Err != nil {
				fmt.Fprintf(w, "event: error\ndata: Could not reach PubChem — please try again.\n\n")
				flusher.Flush()
				return
			}

			summary := batchSummary{Total: len(snap.Results)}
			hasRoles := false
			for _, res := range snap.Results {
				switch {
				case res.Error == "Not found in PubChem":
					summary.NotFound++
				case res.Error != "":
					summary.Errors++
				default:
					summary.Found++
				}
				if res.Role != "" {
					hasRoles = true
				}
			}
			jsonBytes, err := json.Marshal(snap.Results)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: Failed to encode results.\n\n")
				flusher.Flush()
				return
			}
			data := batchData{
				Results:     snap.Results,
				ResultsJSON: string(jsonBytes),
				Summary:     summary,
				SystemID:    h.resolver.SystemID(),
				HasRoles:    hasRoles,
			}
			var buf bytes.Buffer
			if err := h.tmpl.ExecuteTemplate(&buf, "batch_result.html", data); err != nil {
				fmt.Fprintf(w, "event: error\ndata: Failed to render results.\n\n")
				flusher.Flush()
				return
			}
			// SSE data fields cannot contain bare newlines — collapse to spaces.
			html := bytes.ReplaceAll(buf.Bytes(), []byte("\n"), []byte(" "))
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", html)
			flusher.Flush()
			return
		}
	}
}
