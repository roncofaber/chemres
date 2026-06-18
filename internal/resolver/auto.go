package resolver

import (
	"strings"
	"sync"
	"time"
)

// looksLikeSMILES returns true when the input is confidently a SMILES string.
// Requires explicit SMILES notation chars OR a pure SMILES-alphabet sequence
// of length > 2 containing at least one organic element.
func looksLikeSMILES(s string) bool {
	if strings.ContainsAny(s, "()=[#@/\\") {
		return true // unambiguous SMILES notation
	}
	if len([]rune(s)) <= 2 || strings.ContainsAny(s, " \t") {
		return false
	}
	for _, c := range s {
		switch c {
		case 'B', 'C', 'N', 'O', 'P', 'S', 'F', 'I', 'H',
			'b', 'c', 'n', 'o', 'p', 's',
			'r', 'l', // Br, Cl
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'.', '+', '-', ':', '%', '[', ']':
		default:
			return false // char not in SMILES alphabet
		}
	}
	return strings.ContainsAny(s, "CNOSPFIbcnosp")
}

// hasNonSmilesChar returns true when the input contains at least one character
// that cannot appear in a SMILES string — a strong signal it is a name.
func hasNonSmilesChar(s string) bool {
	for _, c := range s {
		switch c {
		case 'B', 'C', 'N', 'O', 'P', 'S', 'F', 'I', 'H',
			'b', 'c', 'n', 'o', 'p', 's',
			'r', 'l',
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'(', ')', '[', ']', '=', '#', '@', '/', '\\',
			'.', '+', '-', ':', '%', ' ':
		default:
			return true
		}
	}
	return false
}

// AutoResolver routes to the appropriate sub-resolver using a three-tier strategy:
//  1. Confident SMILES  → direct SMILES route
//  2. Confident name    → direct name route
//  3. Ambiguous         → try SMILES first, fall back to name on 400
type AutoResolver struct {
	smiles *SmilesResolver
	name   *NameResolver
}

func NewAutoResolver() Resolver {
	client := newPubchemClient()
	return &AutoResolver{
		smiles: &SmilesResolver{client: client},
		name:   &NameResolver{client: client},
	}
}

func (r *AutoResolver) SystemID() string { return "auto" }
func (r *AutoResolver) Name() string     { return "Chemical Identifier" }

func (r *AutoResolver) resolve(input string, fetchSVG bool) (CompoundResult, error) {
	// Unambiguous exact identifiers
	if casRE.MatchString(input) || inchiKeyRE.MatchString(input) {
		return r.name.resolve(input, fetchSVG)
	}

	// Confident SMILES
	if looksLikeSMILES(input) {
		return r.smiles.resolve(input, fetchSVG)
	}

	// Confident name (has chars outside the SMILES alphabet, or spaces)
	if hasNonSmilesChar(input) {
		return r.name.resolve(input, fetchSVG)
	}

	// Ambiguous (e.g. "CO", "NO") — try SMILES, fall back to name on 400
	result, err := r.smiles.resolve(input, fetchSVG)
	if err == errBadInput {
		return r.name.resolve(input, fetchSVG)
	}
	return result, err
}

func (r *AutoResolver) Resolve(input string) (CompoundResult, error) {
	return r.resolve(input, true)
}

func (r *AutoResolver) Batch(inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}

func (r *AutoResolver) Suggest(query string) ([]string, error) {
	// Suppress autocomplete for SMILES and exact identifiers
	if looksLikeSMILES(query) || casRE.MatchString(query) || inchiKeyRE.MatchString(query) {
		return nil, nil
	}
	return r.name.Suggest(query)
}
