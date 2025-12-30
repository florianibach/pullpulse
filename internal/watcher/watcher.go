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
