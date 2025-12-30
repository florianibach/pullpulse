package web

import (
	"database/sql"
	"net/http"

	"dockerhub-pull-watcher/internal/watcher"
)

type Router struct {
	h *Handlers
}

func NewRouter(db *sql.DB, w *watcher.Service, tpl *Templates) http.Handler {
	h := NewHandlers(db, w, tpl)
	mux := http.NewServeMux()

	mux.HandleFunc("/", h.Home)

	mux.HandleFunc("/targets", h.TargetsListOrCreate)      // GET list, POST create
	mux.HandleFunc("/targets/new", h.TargetNew)            // GET
	mux.HandleFunc("/targets/edit", h.TargetEditOrUpdate)  // GET?id=, POST update

	mux.HandleFunc("/repos", h.ReposList)                  // GET
	mux.HandleFunc("/repo", h.RepoDetail)                  // GET?repo_id=

	return mux
}