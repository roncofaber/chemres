package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/roncofaber/chemres/internal/resolver"
)

type ExportHandler struct {
	tmpl *template.Template
}

func NewExportHandler(tmpl *template.Template) *ExportHandler {
	return &ExportHandler{tmpl: tmpl}
}

func (h *ExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var results []resolver.CompoundResult
	if err := json.Unmarshal([]byte(r.FormValue("results")), &results); err != nil {
		http.Error(w, "invalid results data", http.StatusBadRequest)
		return
	}
	system := r.FormValue("system")
	if system == "" {
		system = "chem"
	}
	filename := fmt.Sprintf("%s_%s.csv", system, time.Now().UTC().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	cw := csv.NewWriter(w)
	cw.Write([]string{"Input", "CID", "IUPAC", "Formula", "MW", "CAS", "InChIKey", "CanonicalSMILES", "IsomericSMILES", "Error"})
	for _, res := range results {
		cid := ""
		if res.CID != 0 {
			cid = fmt.Sprintf("%d", res.CID)
		}
		cw.Write([]string{
			res.Input, cid, res.IUPAC, res.Formula, res.MW,
			res.CAS, res.InChIKey, res.Canonical, res.Isomeric, res.Error,
		})
	}
	cw.Flush()
}
