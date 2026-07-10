package handlers

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// BatchEntry is an input identifier with its reaction role.
type BatchEntry struct {
	Input string
	Role  string
}

type cdxmlStep struct {
	Reactants string `xml:"ReactionStepReactants,attr"`
	Products  string `xml:"ReactionStepProducts,attr"`
	Above     string `xml:"ReactionStepObjectsAboveArrow,attr"`
	Below     string `xml:"ReactionStepObjectsBelowArrow,attr"`
}

// parseCDXML extracts batch entries (identifier + role) from raw CDXML bytes.
func parseCDXML(data []byte) ([]BatchEntry, error) {
	smilesMap, err := cdxmlToSMILES(data)
	if err != nil {
		return nil, err
	}

	fragRoles, condTexts := cdxmlRoles(data)

	seen := map[string]bool{}
	var entries []BatchEntry

	for fragID, smiles := range smilesMap {
		role := fragRoles[fragID]
		if role == "" {
			continue
		}
		key := smiles + "\x00" + role
		if !seen[key] {
			seen[key] = true
			entries = append(entries, BatchEntry{Input: smiles, Role: role})
		}
	}

	for _, ct := range condTexts {
		for _, tok := range splitCondText(ct.text) {
			key := tok + "\x00" + ct.role
			if !seen[key] {
				seen[key] = true
				entries = append(entries, BatchEntry{Input: tok, Role: ct.role})
			}
		}
	}

	return entries, nil
}

// cdxmlToSMILES runs obabel on the CDXML data and returns fragmentID → SMILES.
func cdxmlToSMILES(data []byte) (map[string]string, error) {
	f, err := os.CreateTemp("", "chemres-*.cdxml")
	if err != nil {
		return nil, fmt.Errorf("cdxml temp file: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, fmt.Errorf("cdxml write: %w", err)
	}
	f.Close()

	// obabel exits non-zero when some molecules fail; partial output is still useful.
	out, _ := exec.Command("obabel", f.Name(), "-osmi").Output()

	m := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) < 2 {
			continue
		}
		smiles := strings.TrimSpace(parts[0])
		id := strings.TrimSpace(parts[1])
		if smiles == "" || smiles == "*" || id == "" {
			continue
		}
		m[id] = smiles
	}
	return m, nil
}

type condEntry struct{ role, text string }

// cdxmlRoles parses reaction steps and returns:
//   - fragRoles: fragmentID → role string ("Reactant (R1)", "Product (R1)", …)
//   - conditions: condition text entries with roles
func cdxmlRoles(data []byte) (fragRoles map[string]string, conditions []condEntry) {
	fragRoles = make(map[string]string)

	var doc struct {
		XMLName xml.Name    `xml:"CDXML"`
		Steps   []cdxmlStep `xml:"page>scheme>step"`
	}
	xml.Unmarshal(data, &doc) //nolint:errcheck

	for i, step := range doc.Steps {
		rxn := fmt.Sprintf("R%d", i+1)

		for _, id := range splitIDs(step.Reactants) {
			for _, fid := range resolveGroupOrFrag(data, id) {
				fragRoles[fid] = "Reactant (" + rxn + ")"
			}
		}
		for _, id := range splitIDs(step.Products) {
			for _, fid := range resolveGroupOrFrag(data, id) {
				fragRoles[fid] = "Product (" + rxn + ")"
			}
		}
		for _, id := range splitIDs(step.Above) {
			if text := elementText(data, id); text != "" {
				conditions = append(conditions, condEntry{"Reagent (" + rxn + ")", text})
			}
		}
		for _, id := range splitIDs(step.Below) {
			if text := elementText(data, id); text != "" {
				conditions = append(conditions, condEntry{"Condition (" + rxn + ")", text})
			}
		}
	}
	return
}

// resolveGroupOrFrag returns the fragment IDs for an element: if the element is a
// <group>, returns its contained <fragment> IDs; otherwise returns the ID itself.
func resolveGroupOrFrag(data []byte, id string) []string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth, targetDepth := 0, 0
	inTarget := false
	var fragIDs []string

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if !inTarget {
				for _, a := range t.Attr {
					if a.Name.Local == "id" && a.Value == id {
						inTarget = true
						targetDepth = depth
						if t.Name.Local != "group" {
							return []string{id}
						}
					}
				}
			} else if depth == targetDepth+1 && t.Name.Local == "fragment" {
				for _, a := range t.Attr {
					if a.Name.Local == "id" {
						fragIDs = append(fragIDs, a.Value)
					}
				}
			}
		case xml.EndElement:
			if inTarget && depth == targetDepth {
				if len(fragIDs) > 0 {
					return fragIDs
				}
				return []string{id}
			}
			depth--
		}
	}
	return []string{id}
}

// elementText extracts all character data from the subtree of the element with the given ID.
func elementText(data []byte, id string) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth, targetDepth := 0, 0
	inTarget := false
	var parts []string

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if !inTarget {
				for _, a := range t.Attr {
					if a.Name.Local == "id" && a.Value == id {
						inTarget = true
						targetDepth = depth
					}
				}
			}
		case xml.EndElement:
			if inTarget && depth == targetDepth {
				inTarget = false
			}
			depth--
		case xml.CharData:
			if inTarget {
				if s := strings.TrimSpace(string(t)); s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func splitIDs(s string) []string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return strings.Fields(s)
}

var (
	condSplitRE = regexp.MustCompile(`[,;\n]+`)
	condSkipRE  = regexp.MustCompile(`(?i)^\d|°|\d+\s*(h|min|M)\b|^rt$|^reflux$|^overnight$`)
	parenRE     = regexp.MustCompile(`\s*\(.*?\)`)
)

// splitCondText splits a condition string into resolvable chemical name tokens,
// stripping temperature/time/parenthetical qualifiers.
func splitCondText(text string) []string {
	var result []string
	for _, tok := range condSplitRE.Split(text, -1) {
		tok = parenRE.ReplaceAllString(tok, "")
		tok = strings.TrimSpace(tok)
		if len([]rune(tok)) < 2 || condSkipRE.MatchString(tok) {
			continue
		}
		result = append(result, tok)
	}
	return result
}
