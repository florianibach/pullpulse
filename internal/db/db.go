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
