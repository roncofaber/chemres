package handlers

import (
	"html/template"
	"path/filepath"
	"regexp"
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

var funcMap = template.FuncMap{
	"formatFormula":  formatFormula,
	"compoundTitle":  compoundTitle,
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
