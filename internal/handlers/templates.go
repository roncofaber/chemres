package handlers

import (
	"fmt"
	"html/template"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var digitRE = regexp.MustCompile(`\d+`)

func formatFormula(f string) template.HTML {
	return template.HTML(digitRE.ReplaceAllStringFunc(f, func(d string) string {
		return "<sub>" + d + "</sub>"
	}))
}

// compoundTitle returns "Name · Formula" as safe HTML, handling missing fields.
func compoundTitle(name, formula string) template.HTML {
	escaped := template.HTMLEscapeString(name)
	if name != "" && formula != "" {
		escaped += " · "
	}
	if formula != "" {
		escaped += string(formatFormula(formula))
	}
	return template.HTML(escaped)
}

// propFloat formats an optional float64 pointer with the given precision.
// Returns "" if nil, so {{with propFloat .XLogP 2}} skips the block when absent.
func propFloat(p *float64, prec int) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', prec, 64)
}

// propInt formats an optional int pointer.
// Returns "" if nil; "0" is returned for a present-but-zero value.
func propInt(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

// formatHStatement parses "H300 (99.8%): Fatal if swallowed [Danger ...]"
// into three aligned spans: code, percentage, description (bracket dropped).
func formatHStatement(s string) template.HTML {
	// Extract code: up to first space or '('
	code := s
	rest := ""
	if i := strings.IndexAny(s, " ("); i > 0 {
		code = s[:i]
		rest = strings.TrimSpace(s[i:])
	}

	// Extract percentage: optional "(xx%)" or "(> xx%)" before the colon
	pct := ""
	if strings.HasPrefix(rest, "(") {
		end := strings.Index(rest, ")")
		if end > 0 {
			pct = rest[1:end]
			rest = strings.TrimSpace(rest[end+1:])
		}
	}

	// Skip leading colon
	rest = strings.TrimPrefix(rest, ":")
	rest = strings.TrimSpace(rest)

	// Separate bracket category "[Danger ...]" for muted display
	cat := ""
	if i := strings.Index(rest, "["); i > 0 {
		if j := strings.LastIndex(rest, "]"); j > i {
			cat = rest[i : j+1]
		}
		rest = strings.TrimSpace(rest[:i])
	}

	h := `<span class="hcode">` + template.HTMLEscapeString(code) + `</span>`
	if pct != "" {
		h += `<span class="hpct">` + template.HTMLEscapeString(pct) + `</span>`
	}
	h += `<span class="hdesc">` + template.HTMLEscapeString(rest)
	if cat != "" {
		h += ` <span class="hcat">` + template.HTMLEscapeString(cat) + `</span>`
	}
	h += `</span>`
	return template.HTML(h)
}

var funcMap = template.FuncMap{
	"formatFormula":   formatFormula,
	"compoundTitle":   compoundTitle,
	"propFloat":       propFloat,
	"propInt":         propInt,
	"formatHStatement": formatHStatement,
}

func MustLoadTemplates(dir string) *template.Template {
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(filepath.Join(dir, "*.html"))
	if err != nil {
		panic("failed to parse base templates: " + err.Error())
	}
	partials, err := filepath.Glob(filepath.Join(dir, "partials", "*.html"))
	if err != nil {
		panic("failed to glob partials: " + err.Error())
	}
	if len(partials) > 0 {
		if tmpl, err = tmpl.ParseFiles(partials...); err != nil {
			panic("failed to parse partial templates: " + err.Error())
		}
	}
	tabs, err := filepath.Glob(filepath.Join(dir, "*/tab.html"))
	if err != nil {
		panic("failed to glob tab templates: " + err.Error())
	}
	if len(tabs) > 0 {
		if tmpl, err = tmpl.ParseFiles(tabs...); err != nil {
			panic("failed to parse tab templates: " + err.Error())
		}
	}
	return tmpl
}
