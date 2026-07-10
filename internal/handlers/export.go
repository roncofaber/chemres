package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/roncofaber/chemres/internal/resolver"
)

const maxExportJSON = 10 << 20 // 10 MB

// ghsCell collapses GHS data into a single semicolon-separated cell.
func ghsCell(ghs *resolver.GHSData) string {
	if ghs == nil {
		return ""
	}
	hcodes := make([]string, len(ghs.HStatements))
	for i, h := range ghs.HStatements {
		if idx := strings.IndexAny(h, " ("); idx > 0 {
			hcodes[i] = h[:idx]
		} else {
			hcodes[i] = h
		}
	}
	var parts []string
	if ghs.Signal != "" {
		parts = append(parts, "Signal="+ghs.Signal)
	}
	if len(ghs.Pictograms) > 0 {
		parts = append(parts, "Pictograms="+strings.Join(ghs.Pictograms, ","))
	}
	if len(hcodes) > 0 {
		parts = append(parts, "HCodes="+strings.Join(hcodes, ","))
	}
	if ghs.PCodes != "" {
		parts = append(parts, "PCodes="+ghs.PCodes)
	}
	return strings.Join(parts, "; ")
}

// computedPropsCell returns a semicolon-separated string of non-empty computed properties.
func computedPropsCell(r resolver.CompoundResult) string {
	var parts []string
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	add("ExactMass", r.ExactMass)
	add("XLogP", propFloat(r.XLogP, 2))
	add("TPSA", propFloat(r.TPSA, 1))
	add("HBD", propInt(r.HBondDonorCount))
	add("HBA", propInt(r.HBondAcceptorCount))
	add("RotBonds", propInt(r.RotatableBondCount))
	add("Stereocenters", propInt(r.AtomStereoCount))
	add("Charge", propInt(r.Charge))
	add("Volume3D", propFloat(r.Volume3D, 1))
	return strings.Join(parts, "; ")
}

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
	raw := r.FormValue("results")
	if len(raw) > maxExportJSON {
		http.Error(w, "results payload too large", http.StatusBadRequest)
		return
	}
	var results []resolver.CompoundResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		http.Error(w, "invalid results data", http.StatusBadRequest)
		return
	}
	system := r.FormValue("system")
	if system == "" {
		system = "chem"
	}
	filename := fmt.Sprintf("%s_%s.csv", system, time.Now().UTC().Format("20060102_150405"))
	// Quote the filename to prevent header injection.
	filename = strings.ReplaceAll(filename, `"`, "")
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	hasGHS := false
	for _, res := range results {
		if res.GHS != nil {
			hasGHS = true
			break
		}
	}

	cw := csv.NewWriter(w)
	hasRoles := false
	for _, res := range results {
		if res.Role != "" {
			hasRoles = true
			break
		}
	}

	header := []string{"Input"}
	if hasRoles {
		header = append(header, "Role")
	}
	header = append(header,
		"CID", "PubChemURL", "CommonName", "IUPAC", "Formula", "MW",
		"CAS", "InChIKey", "InChI", "CanonicalSMILES", "IsomericSMILES",
		"ComputedProperties",
	)
	if hasGHS {
		header = append(header, "GHS")
	}
	header = append(header, "ResolvedAt", "Error")
	cw.Write(header)

	for _, res := range results {
		cid := ""
		pubchemURL := ""
		if res.CID != 0 {
			cid = fmt.Sprintf("%d", res.CID)
			pubchemURL = fmt.Sprintf("https://pubchem.ncbi.nlm.nih.gov/compound/%d", res.CID)
		}
		resolvedAt := ""
		if !res.ResolvedAt.IsZero() {
			resolvedAt = res.ResolvedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		row := []string{res.Input}
		if hasRoles {
			row = append(row, res.Role)
		}
		row = append(row,
			cid, pubchemURL, res.CommonName, res.IUPAC, res.Formula, res.MW,
			res.CAS, res.InChIKey, res.InChI, res.Canonical, res.Isomeric,
			computedPropsCell(res),
		)
		if hasGHS {
			row = append(row, ghsCell(res.GHS))
		}
		row = append(row, resolvedAt, res.Error)
		cw.Write(row)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Printf("CSV write error: %v", err)
	}
}
