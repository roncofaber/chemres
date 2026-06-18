package resolver

import (
	"strings"
	"sync"
	"time"
)

// looksLikeSMILES returns true if the string looks like a SMILES identifier.
// Strategy: if it has explicit SMILES notation chars → definitely SMILES.
// Otherwise: if all chars are in the SMILES alphabet and none are letters
// that only appear in human-readable names (e.g. 'a','d','e','g','j','k',
// 'm','q','t','u','v','w','x','y','z') → likely SMILES.
// Short strings (≤2 chars) are treated as names to avoid CO/NO ambiguity.
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
			// valid SMILES character — continue
		default:
			return false // contains a letter/char not in the SMILES alphabet
		}
	}
	// Must contain at least one heavy organic atom symbol
	return strings.ContainsAny(s, "CNOSPFIbcnosp")
}

// AutoResolver detects the identifier type and routes to the appropriate
// sub-resolver. Accepts SMILES, names, CAS numbers, and InChIKeys.
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

func (r *AutoResolver) detect(input string) string {
	if casRE.MatchString(input) {
		return "name" // CAS goes via name namespace
	}
	if inchiKeyRE.MatchString(input) {
		return "inchikey"
	}
	if looksLikeSMILES(input) {
		return "smiles"
	}
	return "name"
}

func (r *AutoResolver) resolve(input string, fetchSVG bool) (CompoundResult, error) {
	switch r.detect(input) {
	case "smiles":
		return r.smiles.resolve(input, fetchSVG)
	case "inchikey":
		// NameResolver already handles InChIKey routing internally
		return r.name.resolve(input, fetchSVG)
	default:
		return r.name.resolve(input, fetchSVG)
	}
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
