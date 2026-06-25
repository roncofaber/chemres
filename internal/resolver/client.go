package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const pubchemBase = "https://pubchem.ncbi.nlm.nih.gov/rest/pug"
const pubchemAutocomplete = "https://pubchem.ncbi.nlm.nih.gov/rest/autocomplete"
const propertyFields = "IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES,SMILES,ConnectivitySMILES,Title"

const (
	rateLimitPerSec = 4
	rateBurst       = 4
	maxRetries      = 3
)

var retryBackoffs = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond}

var errNotFound = errors.New("not found")
var errBadInput = errors.New("bad input")
var casRE      = regexp.MustCompile(`^\d+-\d+-\d+$`)
var inchiKeyRE = regexp.MustCompile(`^[A-Z]{14}-[A-Z]{10}-[A-Z]$`)

func firstCommonName(synonyms []string) string {
	for _, s := range synonyms {
		if casRE.MatchString(s) || inchiKeyRE.MatchString(s) {
			continue
		}
		if strings.ContainsRune(s, ':') {
			continue
		}
		return s
	}
	return ""
}

type pubchemClient struct {
	baseURL          string
	autocompleteBase string
	http             *http.Client
	limiters         sync.Map
}

func newPubchemClient() *pubchemClient {
	return &pubchemClient{
		baseURL:          pubchemBase,
		autocompleteBase: pubchemAutocomplete,
		http:             &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *pubchemClient) limiterFor(ip string) *rate.Limiter {
	if ip == "" {
		return nil
	}
	v, _ := c.limiters.LoadOrStore(ip, rate.NewLimiter(rateLimitPerSec, rateBurst))
	return v.(*rate.Limiter)
}

func isTransient(statusCode int, err error) bool {
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return true
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return true
		}
		return false
	}
	if statusCode >= 400 && statusCode < 500 {
		return false
	}
	return statusCode >= 500
}

func (c *pubchemClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if ip := clientIPFromCtx(ctx); ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}

	lim := c.limiterFor(clientIPFromCtx(ctx))
	if lim != nil {
		if err := lim.Wait(ctx); err != nil {
			return nil, err
		}
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("WARN pubchem retry attempt=%d ip=%s", attempt, clientIPFromCtx(ctx))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoffs[attempt-1]):
			}
		}
		resp, err = c.http.Do(req.Clone(ctx))
		if err != nil {
			if isTransient(0, err) {
				continue
			}
			return nil, err
		}
		if isTransient(resp.StatusCode, nil) {
			resp.Body.Close()
			resp = nil
			continue
		}
		return resp, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("pubchem unavailable after %d retries", maxRetries)
}

type propertyRow struct {
	CID                int    `json:"CID"`
	IUPACName          string `json:"IUPACName"`
	MolecularFormula   string `json:"MolecularFormula"`
	MolecularWeight    string `json:"MolecularWeight"`
	InChIKey           string `json:"InChIKey"`
	CanonicalSMILES    string `json:"CanonicalSMILES"`
	IsomericSMILES     string `json:"IsomericSMILES"`
	SMILES             string `json:"SMILES"`
	ConnectivitySMILES string `json:"ConnectivitySMILES"`
	Title              string `json:"Title"`
}

type propertyTable struct {
	Properties []propertyRow `json:"Properties"`
}

type propertyResponse struct {
	PropertyTable propertyTable `json:"PropertyTable"`
}

type synonymEntry struct {
	CID     int      `json:"CID"`
	Synonym []string `json:"Synonym"`
}

type synonymInfo struct {
	Information []synonymEntry `json:"Information"`
}

type synonymResponse struct {
	InformationList synonymInfo `json:"InformationList"`
}

func (c *pubchemClient) fetchProperties(ctx context.Context, namespace, identifier string, namespaceIsSmiles bool) (propertyResponse, error) {
	path := fmt.Sprintf("%s/compound/%s/property/%s/JSON", c.baseURL, namespace, propertyFields)

	var req *http.Request
	var err error
	if namespaceIsSmiles {
		body := strings.NewReader(url.Values{"smiles": {identifier}}.Encode())
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, path, body)
		if err != nil {
			return propertyResponse{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		getURL := fmt.Sprintf("%s/compound/%s/%s/property/%s/JSON",
			c.baseURL, namespace, url.PathEscape(identifier), propertyFields)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
		if err != nil {
			return propertyResponse{}, err
		}
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return propertyResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return propertyResponse{}, errNotFound
	}
	if resp.StatusCode == http.StatusBadRequest {
		return propertyResponse{}, errBadInput
	}
	if resp.StatusCode != http.StatusOK {
		return propertyResponse{}, fmt.Errorf("pubchem returned %d", resp.StatusCode)
	}
	var result propertyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return propertyResponse{}, fmt.Errorf("decoding property response: %w", err)
	}
	return result, nil
}


func (c *pubchemClient) fetchSynonyms(ctx context.Context, cid int) (cas string, synonyms []string, err error) {
	u := fmt.Sprintf("%s/compound/cid/%d/synonyms/JSON", c.baseURL, cid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("synonyms fetch returned %d", resp.StatusCode)
	}
	var sr synonymResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", nil, err
	}
	if len(sr.InformationList.Information) == 0 {
		return "", nil, nil
	}
	synonyms = sr.InformationList.Information[0].Synonym
	for _, syn := range synonyms {
		if casRE.MatchString(syn) {
			cas = syn
			break
		}
	}
	return cas, synonyms, nil
}

// fetchPropertiesBatch fetches properties for a list of CIDs in one request.
// Returns a map of CID → propertyRow.
func (c *pubchemClient) fetchPropertiesBatch(ctx context.Context, cids []int) (map[int]propertyRow, error) {
	if len(cids) == 0 {
		return nil, nil
	}
	cidStrs := make([]string, len(cids))
	for i, cid := range cids {
		cidStrs[i] = strconv.Itoa(cid)
	}
	u := fmt.Sprintf("%s/compound/cid/%s/property/%s/JSON",
		c.baseURL, strings.Join(cidStrs, ","), propertyFields)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch properties fetch returned %d", resp.StatusCode)
	}
	var result propertyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	m := make(map[int]propertyRow, len(result.PropertyTable.Properties))
	for _, p := range result.PropertyTable.Properties {
		m[p.CID] = p
	}
	return m, nil
}

type synonymBatchEntry struct {
	CAS      string
	Synonyms []string
}

// fetchSynonymsBatch fetches synonyms for a list of CIDs in one request.
func (c *pubchemClient) fetchSynonymsBatch(ctx context.Context, cids []int) (map[int]synonymBatchEntry, error) {
	if len(cids) == 0 {
		return nil, nil
	}
	cidStrs := make([]string, len(cids))
	for i, cid := range cids {
		cidStrs[i] = strconv.Itoa(cid)
	}
	u := fmt.Sprintf("%s/compound/cid/%s/synonyms/JSON",
		c.baseURL, strings.Join(cidStrs, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch synonyms fetch returned %d", resp.StatusCode)
	}
	var sr synonymResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
	m := make(map[int]synonymBatchEntry, len(sr.InformationList.Information))
	for _, entry := range sr.InformationList.Information {
		var cas string
		for _, syn := range entry.Synonym {
			if casRE.MatchString(syn) {
				cas = syn
				break
			}
		}
		m[entry.CID] = synonymBatchEntry{CAS: cas, Synonyms: entry.Synonym}
	}
	return m, nil
}

type autocompleteResponse struct {
	DictionaryTerms struct {
		Compound []string `json:"compound"`
	} `json:"dictionary_terms"`
}

func (c *pubchemClient) autocomplete(ctx context.Context, prefix string, limit int) ([]string, error) {
	u := fmt.Sprintf("%s/compound/%s/JSON?limit=%d", c.autocompleteBase, url.PathEscape(prefix), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var result autocompleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.DictionaryTerms.Compound, nil
}
