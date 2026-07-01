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
	header := []string{"Input", "CID", "PubChemURL", "IUPAC", "CommonName", "Formula", "MW", "ExactMass", "CAS", "InChIKey", "InChI", "CanonicalSMILES", "IsomericSMILES", "XLogP", "TPSA", "HBondDonors", "HBondAcceptors", "RotatableBonds", "AtomStereoCount", "Charge", "Volume3D", "ResolvedAt", "Error"}
	if hasGHS {
		header = append(header, "GHSSignal", "GHSPictograms", "GHSHazardCodes", "GHSPrecautionaryCodes")
	}
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
		row := []string{
			res.Input, cid, pubchemURL, res.IUPAC, res.CommonName, res.Formula, res.MW,
			res.ExactMass, res.CAS, res.InChIKey, res.InChI, res.Canonical, res.Isomeric,
			propFloat(res.XLogP, 2), propFloat(res.TPSA, 1),
			propInt(res.HBondDonorCount), propInt(res.HBondAcceptorCount),
			propInt(res.RotatableBondCount), propInt(res.AtomStereoCount),
			propInt(res.Charge), propFloat(res.Volume3D, 1),
			resolvedAt, res.Error,
		}
		if hasGHS {
			if res.GHS != nil {
				hcodes := make([]string, len(res.GHS.HStatements))
				for i, h := range res.GHS.HStatements {
					if idx := strings.IndexAny(h, " ("); idx > 0 {
						hcodes[i] = h[:idx]
					} else {
						hcodes[i] = h
					}
				}
				row = append(row,
					res.GHS.Signal,
					strings.Join(res.GHS.Pictograms, ";"),
					strings.Join(hcodes, ";"),
					res.GHS.PCodes,
				)
			} else {
				row = append(row, "", "", "", "")
			}
		}
		cw.Write(row)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Printf("CSV write error: %v", err)
	}
}
