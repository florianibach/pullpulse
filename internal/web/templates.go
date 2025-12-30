package web

import (
	"html/template"
	"path/filepath"
)

type Templates struct {
	Base *template.Template
}

func LoadTemplates(dir string) (*Templates, error) {
	pattern := filepath.Join(dir, "*.html")
	t, err := template.New("base").ParseGlob(pattern)
	if err != nil {
		return nil, err
	}
	return &Templates{Base: t}, nil
}