package resolver

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// looksLikeSMILES returns true when the input is confidently a SMILES string.
// Requires explicit SMILES notation chars OR a pure SMILES-alphabet sequence
// of length > 2 containing at least one organic element.
func looksLikeSMILES(s string) bool {
	if strings.ContainsAny(s, " \t") {
		return false // SMILES never contain spaces
	}
	if strings.ContainsAny(s, "()=[#@/\\") {
		return true // unambiguous SMILES notation
	}
	if len([]rune(s)) <= 2 {
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

func (r *AutoResolver) resolve(ctx context.Context, input string, withSynonyms bool) (CompoundResult, error) {
	if casRE.MatchString(input) || inchiKeyRE.MatchString(input) {
		return r.name.resolve(ctx, input, withSynonyms)
	}
	if strings.HasPrefix(input, "InChI=") {
		return r.name.resolve(ctx, input, withSynonyms)
	}
	if looksLikeSMILES(input) {
		result, err := r.smiles.resolve(ctx, input, withSynonyms)
		if err == errBadInput {
			return r.name.resolve(ctx, input, withSynonyms)
		}
		return result, err
	}
	if hasNonSmilesChar(input) {
		return r.name.resolve(ctx, input, withSynonyms)
	}
	result, err := r.smiles.resolve(ctx, input, withSynonyms)
	if err == errBadInput {
		return r.name.resolve(ctx, input, withSynonyms)
	}
	return result, err
}

func (r *AutoResolver) Resolve(ctx context.Context, input string) (CompoundResult, error) {
	return r.resolve(ctx, input, true)
}

func (r *AutoResolver) BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup
	var doneCount int32

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(ctx, in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
			n := int(atomic.AddInt32(&doneCount, 1))
			onResolve(n, len(inputs))
		}(i, input)
	}
	wg.Wait()

	var cids []int
	cidIdx := make(map[int][]int)
	for i, res := range results {
		if res.CID != 0 {
			if _, seen := cidIdx[res.CID]; !seen {
				cids = append(cids, res.CID)
			}
			cidIdx[res.CID] = append(cidIdx[res.CID], i)
		}
	}

	if len(cids) > 0 {
		synMap, err := r.name.client.fetchSynonymsBatch(ctx, cids)
		if err != nil {
			log.Printf("WARN fetchSynonymsBatch: %v", err)
		}
		if err == nil && synMap != nil {
			for cid, idxs := range cidIdx {
				entry, ok := synMap[cid]
				if !ok {
					continue
				}
				for _, i := range idxs {
					results[i].CAS     = entry.CAS
					results[i].Synonyms = entry.Synonyms
					if results[i].CommonName == "" {
						results[i].CommonName = firstCommonName(entry.Synonyms)
					}
				}
			}
		}
	}

	return results, nil
}

func (r *AutoResolver) Batch(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	return r.BatchWithProgress(ctx, inputs, func(_, _ int) {})
}

func (r *AutoResolver) Suggest(ctx context.Context, query string) ([]string, error) {
	// Suppress autocomplete for SMILES and exact identifiers
	if looksLikeSMILES(query) || casRE.MatchString(query) || inchiKeyRE.MatchString(query) {
		return nil, nil
	}
	return r.name.Suggest(ctx, query)
}
