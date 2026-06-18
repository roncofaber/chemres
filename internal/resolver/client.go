package resolver

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const pubchemBase          = "https://pubchem.ncbi.nlm.nih.gov/rest/pug"
const pubchemAutocomplete  = "https://pubchem.ncbi.nlm.nih.gov/rest/autocomplete"
const propertyFields       = "IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES,SMILES,ConnectivitySMILES"

var errNotFound = errors.New("not found")
var errBadInput  = errors.New("bad input")
var casRE = regexp.MustCompile(`^\d+-\d+-\d+$`)

type pubchemClient struct {
	baseURL       string
	autocompleteBase string
	http          *http.Client
}

func newPubchemClient() *pubchemClient {
	return &pubchemClient{
		baseURL:          pubchemBase,
		autocompleteBase: pubchemAutocomplete,
		http:             &http.Client{Timeout: 15 * time.Second},
	}
}

type propertyRow struct {
	CID                  int    `json:"CID"`
	IUPACName            string `json:"IUPACName"`
	MolecularFormula     string `json:"MolecularFormula"`
	MolecularWeight      string `json:"MolecularWeight"`
	InChIKey             string `json:"InChIKey"`
	CanonicalSMILES      string `json:"CanonicalSMILES"`
	IsomericSMILES       string `json:"IsomericSMILES"`
	SMILES               string `json:"SMILES"`
	ConnectivitySMILES   string `json:"ConnectivitySMILES"`
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

// fetchProperties calls PubChem for compound properties.
// SMILES uses POST to handle special characters; name/CAS uses GET.
func (c *pubchemClient) fetchProperties(namespace, identifier string, namespaceIsSmiles bool) (propertyResponse, error) {
	path := fmt.Sprintf("%s/compound/%s/property/%s/JSON", c.baseURL, namespace, propertyFields)

	var (
		resp *http.Response
		err  error
	)
	if namespaceIsSmiles {
		body := strings.NewReader(url.Values{"smiles": {identifier}}.Encode())
		resp, err = c.http.Post(path, "application/x-www-form-urlencoded", body)
	} else {
		getURL := fmt.Sprintf("%s/compound/%s/%s/property/%s/JSON",
			c.baseURL, namespace, url.PathEscape(identifier), propertyFields)
		resp, err = c.http.Get(getURL)
	}
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

// fetchSVG fetches the 2D structure SVG for a CID. Failure is non-fatal.
func (c *pubchemClient) fetchSVG(cid int) (template.HTML, error) {
	u := fmt.Sprintf("%s/compound/cid/%d/record/SVG?record_type=2d", c.baseURL, cid)
	resp, err := c.http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("svg fetch returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return template.HTML(b), nil
}

// fetchSynonyms returns all synonyms and the first CAS number for a CID.
func (c *pubchemClient) fetchSynonyms(cid int) (cas string, synonyms []string, err error) {
	u := fmt.Sprintf("%s/compound/cid/%d/synonyms/JSON", c.baseURL, cid)
	resp, err := c.http.Get(u)
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

type autocompleteResponse struct {
	DictionaryTerms struct {
		Compound []string `json:"compound"`
	} `json:"dictionary_terms"`
}

// autocomplete returns name suggestions for the given prefix.
func (c *pubchemClient) autocomplete(prefix string, limit int) ([]string, error) {
	u := fmt.Sprintf("%s/compound/%s/JSON?limit=%d", c.autocompleteBase, url.PathEscape(prefix), limit)
	resp, err := c.http.Get(u)
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
