package handlers

import (
	"bufio"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

const maxBatchFileSize = 5 << 20

type BatchHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewBatchHandler(tmpl *template.Template, r resolver.Resolver) *BatchHandler {
	return &BatchHandler{tmpl: tmpl, resolver: r}
}

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

func (h *BatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	} else {
		file, _, err := r.FormFile("file")
		if err != nil {
			msg := "Please provide inputs via the text area or upload a file."
			if err.Error() == "http: request body too large" {
				msg = "File too large — maximum size is 5 MB."
			}
			h.tmpl.ExecuteTemplate(w, "batch_result.html", batchData{Error: msg})
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
	}

	if len(inputs) == 0 {
		h.tmpl.ExecuteTemplate(w, "batch_result.html", batchData{
			Error: "No valid inputs found — enter one per line.",
		})
		return
	}

	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	results, _ := h.resolver.Batch(ctx, inputs)

	summary := batchSummary{Total: len(results)}
	for _, res := range results {
		switch {
		case res.Error == "Not found in PubChem":
			summary.NotFound++
		case res.Error != "":
			summary.Errors++
		default:
			summary.Found++
		}
	}

	jsonBytes, _ := json.Marshal(results)
	h.tmpl.ExecuteTemplate(w, "batch_result.html", batchData{
		Results:     results,
		ResultsJSON: string(jsonBytes),
		Summary:     summary,
		SystemID:    h.resolver.SystemID(),
	})
}
