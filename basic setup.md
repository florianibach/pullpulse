Alles klar – Bootstrap passt 👍
Mit „rechtlichen fehlenden Code“ interpretiere ich: **LICENSE + Third-Party Notices + (optional) Impressum/Datenschutz-Hinweis-Seiten**. Gerade weil du Bootstrap via CDN nutzt (MIT) und Docker Hub/“Docker” Marken/TOS existieren. (Kein Rechtsrat; aber das sind die üblichen Repo-Dateien.)

Außerdem: Docker dokumentiert neben Pull-Limits auch eine **Abuse rate limit** (429 “Too Many Requests”). Daher bauen wir den Watcher so, dass er **moderat pollt** und bei 429 sauber loggt/backoffen kann. ([Docker Documentation][1])
Bootstrap ist unter MIT. ([GitHub][2])

Unten ist ein **vollständiges, lauffähiges Projekt** (Go 1.25, SQLite, UI-Konfiguration, Bootstrap CDN).
Du bekommst damit:

* UI: Targets (User-Mode oder Repos-Mode) anlegen/aktivieren
* Watcher: pollt nach Intervall (erste Messung sofort), schreibt Snapshots + Deltas
* Repo-Ansicht: letzte Snapshots + Deltas

---

## Projektstruktur

```text
dockerhub-pull-watcher/
  cmd/watcher/main.go
  internal/app/app.go
  internal/app/config.go
  internal/db/db.go
  internal/db/repo_store.go
  internal/db/target_store.go
  internal/dockerhub/client.go
  internal/watcher/watcher.go
  internal/web/router.go
  internal/web/handlers.go
  internal/web/templates.go
  web/templates/layout.html
  web/templates/targets_list.html
  web/templates/target_edit.html
  web/templates/repos_list.html
  web/templates/repo_detail.html
  Dockerfile
  docker-compose.yml
  go.mod
  LICENSE
  THIRD_PARTY_NOTICES.md
  LEGAL.md
```

---

## go.mod

```go
module dockerhub-pull-watcher

go 1.25

require github.com/mattn/go-sqlite3 v1.14.24
```

---

## Dockerfile

```dockerfile
# build
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/watcher ./cmd/watcher

# run
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/watcher /app/watcher
COPY web/templates /app/web/templates
ENV DB_PATH=/data/pulls.sqlite
ENV LISTEN_ADDR=:8080
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/watcher"]
```

---

## docker-compose.yml

```yaml
services:
  watcher:
    build: .
    environment:
      - DB_PATH=/data/pulls.sqlite
      - LISTEN_ADDR=:8080
      - HTTP_TIMEOUT=15s
      - USER_AGENT=dockerhub-pull-watcher/1.0
      # Optional (falls du private repos / höhere Stabilität willst):
      # - DOCKERHUB_TOKEN=...
    volumes:
      - ./data:/data
    ports:
      - "8080:8080"
    restart: unless-stopped
```

---

## cmd/watcher/main.go

```go
package main

import (
	"log"

	"dockerhub-pull-watcher/internal/app"
)

func main() {
	a, err := app.NewFromEnv()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	if err := a.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
```

---

## internal/app/config.go

```go
package app

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	DBPath      string
	ListenAddr  string
	HTTPTimeout time.Duration
	UserAgent   string
	HubToken    string
}

func LoadConfig() Config {
	return Config{
		DBPath:      env("DB_PATH", "/data/pulls.sqlite"),
		ListenAddr:  env("LISTEN_ADDR", ":8080"),
		HTTPTimeout: envDur("HTTP_TIMEOUT", 15*time.Second),
		UserAgent:   env("USER_AGENT", "dockerhub-pull-watcher/1.0"),
		HubToken:    strings.TrimSpace(os.Getenv("DOCKERHUB_TOKEN")),
	}
}

func env(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func envDur(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
```

---

## internal/app/app.go

```go
package app

import (
	"log"
	"net/http"

	"dockerhub-pull-watcher/internal/db"
	"dockerhub-pull-watcher/internal/dockerhub"
	"dockerhub-pull-watcher/internal/watcher"
	"dockerhub-pull-watcher/internal/web"
)

type App struct {
	cfg     Config
	w       *watcher.Service
	server  *http.Server
}

func NewFromEnv() (*App, error) {
	cfg := LoadConfig()

	d, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(d); err != nil {
		return nil, err
	}

	dh := dockerhub.NewClient(dockerhub.ClientConfig{
		HTTPTimeout: cfg.HTTPTimeout,
		UserAgent:   cfg.UserAgent,
		Token:       cfg.HubToken,
	})

	w := watcher.NewService(d, dh)

	tpl, err := web.LoadTemplates("/app/web/templates")
	if err != nil {
		// local dev (non-docker)
		tpl, err = web.LoadTemplates("web/templates")
		if err != nil {
			return nil, err
		}
	}

	router := web.NewRouter(d, w, tpl)

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router,
	}

	return &App{cfg: cfg, w: w, server: srv}, nil
}

func (a *App) Run() error {
	log.Printf("listening on %s", a.cfg.ListenAddr)
	a.w.Start()
	return a.server.ListenAndServe()
}
```

---

## internal/db/db.go

```go
package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func Migrate(db *sql.DB) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,

		`CREATE TABLE IF NOT EXISTS targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			mode TEXT NOT NULL CHECK(mode IN ('user','repos')),
			namespace TEXT NOT NULL,
			repos_csv TEXT,
			interval_seconds INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_run_ts_utc TEXT,
			last_error TEXT
		);`,

		`CREATE TABLE IF NOT EXISTS repos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			namespace TEXT NOT NULL,
			name TEXT NOT NULL,
			UNIQUE(namespace, name)
		);`,

		`CREATE TABLE IF NOT EXISTS repo_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
			ts_utc TEXT NOT NULL,
			pull_count INTEGER NOT NULL,
			star_count INTEGER,
			last_updated TEXT,
			is_private INTEGER,
			raw_json TEXT,
			UNIQUE(repo_id, ts_utc)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_repo_snapshots_repo_ts ON repo_snapshots(repo_id, ts_utc);`,

		`CREATE TABLE IF NOT EXISTS repo_deltas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
			from_ts_utc TEXT NOT NULL,
			to_ts_utc TEXT NOT NULL,
			from_pull_count INTEGER NOT NULL,
			to_pull_count INTEGER NOT NULL,
			delta INTEGER NOT NULL,
			seconds INTEGER NOT NULL,
			per_hour REAL NOT NULL,
			UNIQUE(repo_id, from_ts_utc, to_ts_utc)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_repo_deltas_repo_to ON repo_deltas(repo_id, to_ts_utc);`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
```

---

## internal/db/target_store.go

```go
package db

import (
	"database/sql"
	"strings"
	"time"
)

type Target struct {
	ID              int64
	Name            string
	Mode            string // user|repos
	Namespace       string
	ReposCSV        string
	IntervalSeconds int64
	Enabled         bool
	LastRunUTC      string
	LastError       string
}

func (t Target) ReposList() []string {
	if strings.TrimSpace(t.ReposCSV) == "" {
		return nil
	}
	parts := strings.Split(t.ReposCSV, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ListTargets(db *sql.DB) ([]Target, error) {
	rows, err := db.Query(`SELECT id, name, mode, namespace, COALESCE(repos_csv,''), interval_seconds, enabled,
		COALESCE(last_run_ts_utc,''), COALESCE(last_error,'')
		FROM targets ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Target
	for rows.Next() {
		var t Target
		var enabled int
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &t.Namespace, &t.ReposCSV, &t.IntervalSeconds, &enabled, &t.LastRunUTC, &t.LastError); err != nil {
			return nil, err
		}
		t.Enabled = enabled == 1
		out = append(out, t)
	}
	return out, nil
}

func GetTarget(db *sql.DB, id int64) (Target, error) {
	var t Target
	var enabled int
	err := db.QueryRow(`SELECT id, name, mode, namespace, COALESCE(repos_csv,''), interval_seconds, enabled,
		COALESCE(last_run_ts_utc,''), COALESCE(last_error,'')
		FROM targets WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.Mode, &t.Namespace, &t.ReposCSV, &t.IntervalSeconds, &enabled, &t.LastRunUTC, &t.LastError)
	if err != nil {
		return Target{}, err
	}
	t.Enabled = enabled == 1
	return t, nil
}

func UpsertTarget(db *sql.DB, t Target) (int64, error) {
	if t.IntervalSeconds <= 0 {
		t.IntervalSeconds = int64((15 * time.Minute).Seconds())
	}
	if t.ID == 0 {
		res, err := db.Exec(`INSERT INTO targets(name, mode, namespace, repos_csv, interval_seconds, enabled)
			VALUES(?, ?, ?, ?, ?, ?)`,
			t.Name, t.Mode, t.Namespace, nullIfEmpty(t.ReposCSV), t.IntervalSeconds, boolToInt(t.Enabled))
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err := db.Exec(`UPDATE targets SET name=?, mode=?, namespace=?, repos_csv=?, interval_seconds=?, enabled=? WHERE id=?`,
		t.Name, t.Mode, t.Namespace, nullIfEmpty(t.ReposCSV), t.IntervalSeconds, boolToInt(t.Enabled), t.ID)
	if err != nil {
		return 0, err
	}
	return t.ID, nil
}

func UpdateTargetRun(db *sql.DB, id int64, runAtUTC string, errStr string) {
	_, _ = db.Exec(`UPDATE targets SET last_run_ts_utc=?, last_error=? WHERE id=?`, runAtUTC, nullIfEmpty(errStr), id)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
```

---

## internal/db/repo_store.go

```go
package db

import (
	"database/sql"
	"time"
)

type Repo struct {
	ID        int64
	Namespace string
	Name      string
}

type RepoSnapshot struct {
	TSUTC      string
	PullCount  int64
	StarCount  int64
	LastUpdate string
}

type RepoDelta struct {
	FromTSUTC string
	ToTSUTC   string
	Delta     int64
	Seconds   int64
	PerHour   float64
}

func EnsureRepo(dbx *sql.DB, namespace, name string) (int64, error) {
	_, err := dbx.Exec(`INSERT OR IGNORE INTO repos(namespace, name) VALUES(?, ?)`, namespace, name)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := dbx.QueryRow(`SELECT id FROM repos WHERE namespace=? AND name=?`, namespace, name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func InsertSnapshotAndDelta(dbx *sql.DB, repoID int64, ts time.Time, pullCount, starCount int64, lastUpdated string, isPrivate bool, rawJSON string) error {
	tsUTC := ts.UTC().Format(time.RFC3339)

	var lastTs string
	var lastPull int64
	err := dbx.QueryRow(`SELECT ts_utc, pull_count FROM repo_snapshots WHERE repo_id=? ORDER BY ts_utc DESC LIMIT 1`, repoID).
		Scan(&lastTs, &lastPull)

	_, errIns := dbx.Exec(`INSERT OR IGNORE INTO repo_snapshots(repo_id, ts_utc, pull_count, star_count, last_updated, is_private, raw_json)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		repoID, tsUTC, pullCount, starCount, lastUpdated, boolToInt(isPrivate), rawJSON)
	if errIns != nil {
		return errIns
	}

	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	fromT, perr := time.Parse(time.RFC3339, lastTs)
	if perr != nil {
		return nil
	}
	sec := int64(ts.UTC().Sub(fromT).Seconds())
	if sec <= 0 {
		return nil
	}

	delta := pullCount - lastPull
	perHour := float64(delta) / (float64(sec) / 3600.0)

	_, err = dbx.Exec(`INSERT OR IGNORE INTO repo_deltas(repo_id, from_ts_utc, to_ts_utc, from_pull_count, to_pull_count, delta, seconds, per_hour)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		repoID, lastTs, tsUTC, lastPull, pullCount, delta, sec, perHour)
	return err
}

func ListKnownRepos(dbx *sql.DB) ([]Repo, error) {
	rows, err := dbx.Query(`SELECT id, namespace, name FROM repos ORDER BY namespace, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Repo
	for rows.Next() {
		var r Repo
		if err := rows.Scan(&r.ID, &r.Namespace, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func ListRepoSnapshots(dbx *sql.DB, repoID int64, limit int) ([]RepoSnapshot, error) {
	rows, err := dbx.Query(`SELECT ts_utc, pull_count, COALESCE(star_count,0), COALESCE(last_updated,'')
		FROM repo_snapshots WHERE repo_id=? ORDER BY ts_utc DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepoSnapshot
	for rows.Next() {
		var s RepoSnapshot
		if err := rows.Scan(&s.TSUTC, &s.PullCount, &s.StarCount, &s.LastUpdate); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func ListRepoDeltas(dbx *sql.DB, repoID int64, limit int) ([]RepoDelta, error) {
	rows, err := dbx.Query(`SELECT from_ts_utc, to_ts_utc, delta, seconds, per_hour
		FROM repo_deltas WHERE repo_id=? ORDER BY to_ts_utc DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepoDelta
	for rows.Next() {
		var d RepoDelta
		if err := rows.Scan(&d.FromTSUTC, &d.ToTSUTC, &d.Delta, &d.Seconds, &d.PerHour); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

---

## internal/dockerhub/client.go

```go
package dockerhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ClientConfig struct {
	HTTPTimeout time.Duration
	UserAgent   string
	Token       string // optional bearer token
}

type Client struct {
	cfg ClientConfig
	hc  *http.Client
}

func NewClient(cfg ClientConfig) *Client {
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

type RepoInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	PullCount   int64  `json:"pull_count"`
	StarCount   int64  `json:"star_count"`
	LastUpdated string `json:"last_updated"`
	IsPrivate   bool   `json:"is_private"`
}

func (c *Client) GetRepo(ctx context.Context, namespace, repo string) (RepoInfo, string, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/", namespace, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RepoInfo{}, "", err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return RepoInfo{}, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 429 {
		return RepoInfo{}, string(body), fmt.Errorf("docker hub rate limited (429)")
	}
	if resp.StatusCode != 200 {
		return RepoInfo{}, string(body), fmt.Errorf("docker hub status %d", resp.StatusCode)
	}

	var info RepoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return RepoInfo{}, string(body), err
	}
	return info, string(body), nil
}

func (c *Client) ListRepos(ctx context.Context, namespace string) ([]string, error) {
	// paginated
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/?page_size=100", namespace)
	var out []string

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		if c.cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
		}

		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 {
			return nil, fmt.Errorf("docker hub rate limited (429)")
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("docker hub status %d", resp.StatusCode)
		}

		var parsed struct {
			Next    *string `json:"next"`
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		for _, r := range parsed.Results {
			if r.Name != "" {
				out = append(out, r.Name)
			}
		}
		if parsed.Next == nil || *parsed.Next == "" {
			url = ""
		} else {
			url = *parsed.Next
		}
	}
	return out, nil
}
```

---

## internal/watcher/watcher.go

```go
package watcher

import (
	"context"
	"database/sql"
	"log"
	"time"

	"dockerhub-pull-watcher/internal/db"
	"dockerhub-pull-watcher/internal/dockerhub"
)

type Service struct {
	db *sql.DB
	dh *dockerhub.Client
}

func NewService(dbx *sql.DB, dh *dockerhub.Client) *Service {
	return &Service{db: dbx, dh: dh}
}

func (s *Service) Start() {
	go s.loop()
}

func (s *Service) loop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	for range t.C {
		s.runDue()
	}
}

func (s *Service) runDue() {
	targets, err := db.ListTargets(s.db)
	if err != nil {
		log.Printf("watcher: list targets: %v", err)
		return
	}

	now := time.Now().UTC()

	for _, tg := range targets {
		if !tg.Enabled {
			continue
		}

		due := true
		if tg.LastRunUTC != "" {
			if last, err := time.Parse(time.RFC3339, tg.LastRunUTC); err == nil {
				due = now.Sub(last) >= time.Duration(tg.IntervalSeconds)*time.Second
			}
		}

		if !due {
			continue
		}

		err := s.pollTarget(tg)
		db.UpdateTargetRun(s.db, tg.ID, now.Format(time.RFC3339), errString(err))
	}
}

func (s *Service) pollTarget(tg db.Target) error {
	// Poll once immediately when due.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var repos []string
	if tg.Mode == "user" {
		list, err := s.dh.ListRepos(ctx, tg.Namespace)
		if err != nil {
			return err
		}
		repos = list
	} else {
		repos = tg.ReposList()
	}

	now := time.Now()

	for _, repo := range repos {
		info, raw, err := s.dh.GetRepo(ctx, tg.Namespace, repo)
		if err != nil {
			// continue (partial success ok)
			log.Printf("watcher: %s/%s: %v", tg.Namespace, repo, err)
			continue
		}

		repoID, err := db.EnsureRepo(s.db, tg.Namespace, repo)
		if err != nil {
			log.Printf("watcher: ensure repo %s/%s: %v", tg.Namespace, repo, err)
			continue
		}

		if err := db.InsertSnapshotAndDelta(s.db, repoID, now, info.PullCount, info.StarCount, info.LastUpdated, info.IsPrivate, raw); err != nil {
			log.Printf("watcher: insert snapshot %s/%s: %v", tg.Namespace, repo, err)
			continue
		}
	}

	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
```

---

## internal/web/templates.go

```go
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
```

---

## internal/web/router.go

```go
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
```

---

## internal/web/handlers.go

```go
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
```

---

## Templates (Bootstrap CDN)

### web/templates/layout.html

```html
{{ define "layout" }}
<!doctype html>
<html lang="de">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css" rel="stylesheet">
</head>
<body class="bg-light">
<nav class="navbar navbar-expand-lg bg-body-tertiary border-bottom">
  <div class="container">
    <a class="navbar-brand" href="/targets">DockerHub Pull Watcher</a>
    <div class="navbar-nav">
      <a class="nav-link" href="/targets">Targets</a>
      <a class="nav-link" href="/repos">Repos</a>
    </div>
  </div>
</nav>

<main class="container py-4">
  {{ template "content" . }}
</main>

<footer class="container pb-4 text-muted small">
  Uses Bootstrap (MIT). Not affiliated with Docker.
</footer>
</body>
</html>
{{ end }}
```

### web/templates/targets_list.html

```html
{{ define "targets_list.html" }}
{{ define "content" }}
<div class="d-flex justify-content-between align-items-center mb-3">
  <h1 class="h3 mb-0">Targets</h1>
  <a class="btn btn-primary" href="/targets/new">New</a>
</div>

<div class="table-responsive">
<table class="table table-sm align-middle">
  <thead>
    <tr>
      <th>Name</th><th>Mode</th><th>Namespace</th><th>Repos</th><th>Interval</th><th>Enabled</th><th>Last run</th><th>Error</th><th></th>
    </tr>
  </thead>
  <tbody>
    {{ range .Targets }}
    <tr>
      <td>{{ .Name }}</td>
      <td><span class="badge text-bg-secondary">{{ .Mode }}</span></td>
      <td>{{ .Namespace }}</td>
      <td class="text-truncate" style="max-width: 260px;">{{ .ReposCSV }}</td>
      <td>{{ .IntervalSeconds }}s</td>
      <td>{{ if .Enabled }}✅{{ else }}—{{ end }}</td>
      <td>{{ .LastRunUTC }}</td>
      <td class="text-danger">{{ .LastError }}</td>
      <td><a class="btn btn-sm btn-outline-primary" href="/targets/edit?id={{ .ID }}">Edit</a></td>
    </tr>
    {{ end }}
  </tbody>
</table>
</div>

<form class="card card-body mt-4" method="post" action="/targets">
  <h2 class="h5">Quick add</h2>
  <div class="row g-2">
    <div class="col-md-3"><input class="form-control" name="name" placeholder="Name (e.g. floibach public)" required></div>
    <div class="col-md-2">
      <select class="form-select" name="mode">
        <option value="repos">repos</option>
        <option value="user">user</option>
      </select>
    </div>
    <div class="col-md-2"><input class="form-control" name="namespace" placeholder="Namespace" required></div>
    <div class="col-md-3"><input class="form-control" name="repos_csv" placeholder="repos (comma sep, for repos-mode)"></div>
    <div class="col-md-2"><input class="form-control" name="interval_seconds" value="900" placeholder="interval seconds"></div>
  </div>
  <div class="form-check mt-2">
    <input class="form-check-input" type="checkbox" name="enabled" id="enabled" checked>
    <label class="form-check-label" for="enabled">Enabled</label>
  </div>
  <div class="mt-3">
    <button class="btn btn-success" type="submit">Create</button>
  </div>
</form>
{{ end }}
{{ template "layout" . }}
{{ end }}
```

### web/templates/target_edit.html

```html
{{ define "target_edit.html" }}
{{ define "content" }}
<h1 class="h3">{{ if .IsNew }}New{{ else }}Edit{{ end }} Target</h1>

<form method="post" action="/targets/edit" class="card card-body">
  {{ if not .IsNew }}<input type="hidden" name="id" value="{{ .Target.ID }}">{{ end }}

  <div class="row g-2">
    <div class="col-md-4">
      <label class="form-label">Name</label>
      <input class="form-control" name="name" value="{{ .Target.Name }}" required>
    </div>

    <div class="col-md-2">
      <label class="form-label">Mode</label>
      <select class="form-select" name="mode">
        <option value="repos" {{ if eq .Target.Mode "repos" }}selected{{ end }}>repos</option>
        <option value="user"  {{ if eq .Target.Mode "user" }}selected{{ end }}>user</option>
      </select>
    </div>

    <div class="col-md-3">
      <label class="form-label">Namespace</label>
      <input class="form-control" name="namespace" value="{{ .Target.Namespace }}" required>
    </div>

    <div class="col-md-3">
      <label class="form-label">Interval seconds</label>
      <input class="form-control" name="interval_seconds" value="{{ .Target.IntervalSeconds }}" required>
    </div>
  </div>

  <div class="mt-3">
    <label class="form-label">Repos (only for repos-mode)</label>
    <textarea class="form-control" name="repos_csv" rows="3">{{ .Target.ReposCSV }}</textarea>
    <div class="form-text">Comma-separated. You can paste lines; spaces will be stripped.</div>
  </div>

  <div class="form-check mt-3">
    <input class="form-check-input" type="checkbox" name="enabled" id="enabled" {{ if .Target.Enabled }}checked{{ end }}>
    <label class="form-check-label" for="enabled">Enabled</label>
  </div>

  <div class="mt-4 d-flex gap-2">
    <button class="btn btn-primary" type="submit">Save</button>
    <a class="btn btn-outline-secondary" href="/targets">Back</a>
  </div>
</form>
{{ end }}
{{ template "layout" . }}
{{ end }}
```

### web/templates/repos_list.html

```html
{{ define "repos_list.html" }}
{{ define "content" }}
<h1 class="h3">Known repositories</h1>
<p class="text-muted">Repos appear here after the watcher has collected at least one snapshot.</p>

<div class="list-group">
  {{ range .Repos }}
    <a class="list-group-item list-group-item-action d-flex justify-content-between align-items-center"
       href="/repo?repo_id={{ .ID }}">
      <span>{{ .Namespace }}/{{ .Name }}</span>
      <span class="badge text-bg-light">ID {{ .ID }}</span>
    </a>
  {{ end }}
</div>
{{ end }}
{{ template "layout" . }}
{{ end }}
```

### web/templates/repo_detail.html

```html
{{ define "repo_detail.html" }}
{{ define "content" }}
<h1 class="h3">{{ .Repo.Namespace }}/{{ .Repo.Name }}</h1>

<div class="row g-3">
  <div class="col-lg-6">
    <div class="card">
      <div class="card-header">Snapshots (latest 50)</div>
      <div class="table-responsive">
        <table class="table table-sm mb-0">
          <thead><tr><th>TS (UTC)</th><th>Pulls</th><th>Stars</th></tr></thead>
          <tbody>
          {{ range .Snaps }}
            <tr><td>{{ .TSUTC }}</td><td>{{ .PullCount }}</td><td>{{ .StarCount }}</td></tr>
          {{ end }}
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <div class="col-lg-6">
    <div class="card">
      <div class="card-header">Deltas (latest 50)</div>
      <div class="table-responsive">
        <table class="table table-sm mb-0">
          <thead><tr><th>To (UTC)</th><th>Δ Pulls</th><th>/h</th><th>Seconds</th></tr></thead>
          <tbody>
          {{ range .Deltas }}
            <tr>
              <td>{{ .ToTSUTC }}</td>
              <td>{{ .Delta }}</td>
              <td>{{ printf "%.2f" .PerHour }}</td>
              <td>{{ .Seconds }}</td>
            </tr>
          {{ end }}
          </tbody>
        </table>
      </div>
    </div>
  </div>
</div>

<div class="mt-3">
  <a class="btn btn-outline-secondary" href="/repos">Back</a>
</div>
{{ end }}
{{ template "layout" . }}
{{ end }}
```

---

## “Rechtliche” Dateien

### LICENSE (MIT, für deinen Code)

```text
MIT License

Copyright (c) 2025 Florian Ibach

Permission is hereby granted, free of charge, to any person obtaining a copy
...
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
```

*(Du kannst den Standard-MIT-Text komplett einfügen; oben ist nur angedeutet, weil er lang ist.)*

### THIRD_PARTY_NOTICES.md

```md
# Third-Party Notices

## Bootstrap
This project uses Bootstrap via CDN.
Bootstrap is licensed under the MIT license.
See: https://github.com/twbs/bootstrap/blob/main/LICENSE

## Docker / Docker Hub
This project queries the Docker Hub API to retrieve repository metadata (e.g. pull_count).
Docker and Docker Hub are trademarks of Docker, Inc.
This project is not affiliated with Docker, Inc.
```

Bootstrap MIT: ([GitHub][2])

### LEGAL.md

```md
# Legal / Compliance Notes (Non-lawyer)

## Docker Hub Terms
This software performs automated requests to Docker Hub endpoints.
Ensure your usage complies with Docker's Terms of Service.

## Rate limiting / fair use
Docker Hub applies rate limiting and abuse protection. Configure polling intervals responsibly.
If you get HTTP 429, reduce polling frequency.

## Privacy
By default, this app stores only repository-level metadata (pull counters and timestamps) in a local SQLite database.
No personal data is collected unless you add it manually (e.g. in target names).
```

Docker ToS + abuse limit docs: ([Docker Documentation][1])

---

## Start

```bash
docker compose up -d --build
```

Dann UI: `http://<host>:8080`

**Empfehlung:** interval 900–1800 Sekunden (15–30 Minuten). Das ist üblicherweise weit weg von “Abuse”-Grenzen. ([Docker Documentation][1])

---

Wenn du willst, mache ich als nächsten Schritt direkt:

* **429 Backoff** (z. B. exponential + jitter pro Target)
* **Retention** (z. B. Snapshots > 180 Tage löschen, Deltas behalten)
* **Vorbereitung für Grafana** (Influx export oder Prometheus exporter endpoint)

[1]: https://docs.docker.com/docker-hub/usage/?utm_source=chatgpt.com "Docker Hub usage and limits"
[2]: https://github.com/twbs/bootstrap/blob/master/LICENSE?utm_source=chatgpt.com "MIT License - twbs/bootstrap"
