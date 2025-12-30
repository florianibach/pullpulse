package app

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"dockerhub-pull-watcher/internal/db"
	"dockerhub-pull-watcher/internal/dockerhub"
	"dockerhub-pull-watcher/internal/watcher"
	"dockerhub-pull-watcher/internal/web"
)

type App struct {
	cfg    Config
	w      *watcher.Service
	server *http.Server
}

func NewFromEnv() (*App, error) {
	cfg := LoadConfig()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, err
	}
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
