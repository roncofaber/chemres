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

// formatHStatement splits "H225: Highly Flammable..." into a bolded code + text.
func formatHStatement(s string) template.HTML {
	if idx := strings.Index(s, ":"); idx > 0 {
		code := template.HTMLEscapeString(s[:idx])
		rest := template.HTMLEscapeString(s[idx+1:])
		return template.HTML("<span class=\"hcode\">" + code + "</span>" + rest)
	}
	return template.HTML(template.HTMLEscapeString(s))
}

var funcMap = template.FuncMap{
	"formatFormula":   formatFormula,
	"compoundTitle":   compoundTitle,
	"propFloat":       propFloat,
	"propInt":         propInt,
	"formatHStatement": formatHStatement,
	"lower":           strings.ToLower,
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
