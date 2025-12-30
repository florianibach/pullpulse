package web

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dockerhub-pull-watcher/internal/db"
	"dockerhub-pull-watcher/internal/watcher"
)

type Handlers struct {
	db  *sql.DB
	w   *watcher.Service
	tpl *Templates
}

func NewHandlers(dbx *sql.DB, w *watcher.Service, tpl *Templates) *Handlers {
	return &Handlers{db: dbx, w: w, tpl: tpl}
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/targets", http.StatusFound)
}

func (h *Handlers) TargetsListOrCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.createTarget(w, r)
		return
	}

	targets, err := db.ListTargets(h.db)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	_ = h.tpl.Base.ExecuteTemplate(w, "targets_list.html", map[string]any{
		"Title":   "Targets",
		"Targets": targets,
	})
}

func (h *Handlers) TargetNew(w http.ResponseWriter, r *http.Request) {
	_ = h.tpl.Base.ExecuteTemplate(w, "target_edit.html", map[string]any{
		"Title":  "New Target",
		"Target": db.Target{Enabled: true, IntervalSeconds: int64((15 * time.Minute).Seconds()), Mode: "repos"},
		"IsNew":  true,
	})
}

func (h *Handlers) TargetEditOrUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.updateTarget(w, r)
		return
	}

	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	t, err := db.GetTarget(h.db, id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	_ = h.tpl.Base.ExecuteTemplate(w, "target_edit.html", map[string]any{
		"Title":  "Edit Target",
		"Target": t,
		"IsNew":  false,
	})
}

func (h *Handlers) createTarget(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	t := parseTargetForm(r)
	_, err := db.UpsertTarget(h.db, t)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/targets", http.StatusFound)
}

func (h *Handlers) updateTarget(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	t := parseTargetForm(r)
	t.ID = id

	_, err := db.UpsertTarget(h.db, t)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/targets", http.StatusFound)
}

func parseTargetForm(r *http.Request) db.Target {
	intervalSec, _ := strconv.ParseInt(r.FormValue("interval_seconds"), 10, 64)
	enabled := r.FormValue("enabled") == "on"

	repos := strings.TrimSpace(r.FormValue("repos_csv"))
	repos = strings.ReplaceAll(repos, "\n", ",")
	repos = strings.ReplaceAll(repos, " ", "")
	repos = strings.Trim(repos, ",")

	return db.Target{
		Name:            strings.TrimSpace(r.FormValue("name")),
		Mode:            strings.TrimSpace(r.FormValue("mode")),
		Namespace:       strings.TrimSpace(r.FormValue("namespace")),
		ReposCSV:        repos,
		IntervalSeconds: intervalSec,
		Enabled:         enabled,
	}
}

func (h *Handlers) ReposList(w http.ResponseWriter, r *http.Request) {
	repos, err := db.ListKnownRepos(h.db)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = h.tpl.Base.ExecuteTemplate(w, "repos_list.html", map[string]any{
		"Title": "Known Repositories",
		"Repos": repos,
	})
}

func (h *Handlers) RepoDetail(w http.ResponseWriter, r *http.Request) {
	repoID, _ := strconv.ParseInt(r.URL.Query().Get("repo_id"), 10, 64)
	repos, _ := db.ListKnownRepos(h.db)

	var selected *db.Repo
	for i := range repos {
		if repos[i].ID == repoID {
			selected = &repos[i]
			break
		}
	}
	if selected == nil {
		http.Redirect(w, r, "/repos", http.StatusFound)
		return
	}

	snaps, err := db.ListRepoSnapshots(h.db, repoID, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	deltas, err := db.ListRepoDeltas(h.db, repoID, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	_ = h.tpl.Base.ExecuteTemplate(w, "repo_detail.html", map[string]any{
		"Title":    selected.Namespace + "/" + selected.Name,
		"Repo":     selected,
		"Snaps":    snaps,
		"Deltas":   deltas,
	})
}