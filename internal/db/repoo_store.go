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
	rows, err := dbx.Query(`SELECT id, namespace, name FROM repos ORDER BY id DESC`)
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