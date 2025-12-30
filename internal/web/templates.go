package web

import (
	"html/template"
	"path/filepath"
)

type Templates struct {
	Dir string
}

func LoadTemplates(dir string) (*Templates, error) {
	return &Templates{Dir: dir}, nil
}

func (t *Templates) Page(name string) (*template.Template, error) {
	// name z.B. "targets_list.html"
	layout := filepath.Join(t.Dir, "layout.html")
	page := filepath.Join(t.Dir, name)
	return template.ParseFiles(layout, page)
}
