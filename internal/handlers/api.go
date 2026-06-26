package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

var unsafeCharsRE = regexp.MustCompile(`[^\w-]+`)

func sanitizeFilename(name string) string {
	return strings.Trim(unsafeCharsRE.ReplaceAllString(name, "_"), "_")
}

const pubchemBase3D = "https://pubchem.ncbi.nlm.nih.gov/rest/pug/compound/cid/%s/SDF?record_type=3d"

// GET /api/v1/conformer?cid=XXX&format=sdf|xyz&name=YYY
func (h *APIHandler) Conformer(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cid := strings.TrimSpace(r.URL.Query().Get("cid"))
	if cid == "" {
		apiError(w, http.StatusBadRequest, "cid is required")
		return
	}
	if _, err := strconv.Atoi(cid); err != nil {
		apiError(w, http.StatusBadRequest, "cid must be a number")
		return
	}
	format := r.URL.Query().Get("format")
	if format != "sdf" && format != "xyz" {
		format = "sdf"
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "compound_CID" + cid
	}
	safe := sanitizeFilename(name)
	if len(safe) > 60 {
		safe = safe[:60]
	}

	resp, err := http.Get(fmt.Sprintf(pubchemBase3D, cid))
	if err != nil {
		apiError(w, http.StatusServiceUnavailable, "could not reach PubChem")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		apiError(w, http.StatusNotFound, "no 3D conformer available for this compound")
		return
	}
	if resp.StatusCode != http.StatusOK {
		apiError(w, http.StatusServiceUnavailable, fmt.Sprintf("PubChem returned %d", resp.StatusCode))
		return
	}
	sdf, err := io.ReadAll(resp.Body)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "failed to read SDF")
		return
	}

	if format == "xyz" {
		xyz, xyzErr := sdfToXyz(string(sdf), name)
		if xyzErr != nil {
			apiError(w, http.StatusInternalServerError, "could not convert SDF to XYZ: "+xyzErr.Error())
			return
		}
		w.Header().Set("Content-Type", "chemical/x-xyz")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xyz"`, safe))
		w.Write([]byte(xyz))
		return
	}

	w.Header().Set("Content-Type", "chemical/x-mdl-sdfile")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.sdf"`, safe))
	w.Write(sdf)
}

func sdfToXyz(sdf, name string) (string, error) {
	lines := strings.Split(sdf, "\n")
	if len(lines) < 5 {
		return "", fmt.Errorf("SDF too short")
	}
	countsLine := lines[3]
	if len(countsLine) < 3 {
		return "", fmt.Errorf("invalid counts line")
	}
	atomCount, err := strconv.Atoi(strings.TrimSpace(countsLine[:3]))
	if err != nil || atomCount <= 0 {
		return "", fmt.Errorf("invalid atom count")
	}
	var atoms []string
	for i := 0; i < atomCount; i++ {
		line := lines[4+i]
		if len(line) < 34 {
			break
		}
		x, _ := strconv.ParseFloat(strings.TrimSpace(line[0:10]), 64)
		y, _ := strconv.ParseFloat(strings.TrimSpace(line[10:20]), 64)
		z, _ := strconv.ParseFloat(strings.TrimSpace(line[20:30]), 64)
		elem := strings.TrimSpace(line[31:34])
		atoms = append(atoms, fmt.Sprintf("%s  %.4f  %.4f  %.4f", elem, x, y, z))
	}
	if len(atoms) == 0 {
		return "", fmt.Errorf("no atoms parsed")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d\n%s\n", len(atoms), name)
	for _, a := range atoms {
		fmt.Fprintln(&b, a)
	}
	return b.String(), nil
}

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
