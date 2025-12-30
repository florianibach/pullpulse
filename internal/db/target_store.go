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

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
