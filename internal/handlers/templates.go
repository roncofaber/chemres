package handlers

import (
	"html/template"
	"path/filepath"
)

func MustLoadTemplates(dir string) *template.Template {
	tmpl, err := template.ParseGlob(filepath.Join(dir, "*.html"))
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
