package state

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type sqliteStore struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS applied_intents (intent_id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS audit (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT, kind TEXT, detail TEXT, prev_hash TEXT, hash TEXT
);
CREATE TABLE IF NOT EXISTS pending_reverts (
  intent_id TEXT PRIMARY KEY,
  kind TEXT, target TEXT, args TEXT, due_at TEXT
);`

// OpenSQLite opens (and migrates) a SQLite-backed Store. Pure-Go driver (no cgo).
func OpenSQLite(path string) (Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) IsIntentApplied(id string) (bool, error) {
	var x string
	err := s.db.QueryRow(`SELECT intent_id FROM applied_intents WHERE intent_id=?`, id).Scan(&x)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *sqliteStore) MarkIntentApplied(id string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO applied_intents(intent_id) VALUES(?)`, id)
	return err
}

func (s *sqliteStore) AppendAudit(kind, detail string) (AuditEntry, error) {
	var prev string
	_ = s.db.QueryRow(`SELECT hash FROM audit ORDER BY seq DESC LIMIT 1`).Scan(&prev)
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	h := ChainHash(ts, kind, detail, prev)
	res, err := s.db.Exec(`INSERT INTO audit(ts,kind,detail,prev_hash,hash) VALUES(?,?,?,?,?)`, ts, kind, detail, prev, h)
	if err != nil {
		return AuditEntry{}, err
	}
	id, _ := res.LastInsertId()
	t, _ := time.Parse(time.RFC3339Nano, ts)
	return AuditEntry{Seq: int(id), TS: t, Kind: kind, Detail: detail, PrevHash: prev, Hash: h}, nil
}

func (s *sqliteStore) Audit() ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT seq,ts,kind,detail,prev_hash,hash FROM audit ORDER BY seq ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts string
		if err := rows.Scan(&e.Seq, &ts, &e.Kind, &e.Detail, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *sqliteStore) VerifyAudit() bool {
	rows, err := s.db.Query(`SELECT ts,kind,detail,prev_hash,hash FROM audit ORDER BY seq ASC`)
	if err != nil {
		return false
	}
	defer rows.Close()
	prev := ""
	for rows.Next() {
		var ts, kind, detail, ph, h string
		if err := rows.Scan(&ts, &kind, &detail, &ph, &h); err != nil {
			return false
		}
		if ph != prev || h != ChainHash(ts, kind, detail, prev) {
			return false
		}
		prev = h
	}
	return true
}

func (s *sqliteStore) ScheduleRevert(r PendingRevert) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO pending_reverts(intent_id,kind,target,args,due_at) VALUES(?,?,?,?,?)`,
		r.IntentID, r.Kind, r.Target, r.ArgsJSON, r.DueAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteStore) DueReverts(now time.Time) ([]PendingRevert, error) {
	rows, err := s.db.Query(`SELECT intent_id,kind,target,args,due_at FROM pending_reverts ORDER BY due_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRevert
	for rows.Next() {
		var r PendingRevert
		var due string
		if err := rows.Scan(&r.IntentID, &r.Kind, &r.Target, &r.ArgsJSON, &due); err != nil {
			return nil, err
		}
		r.DueAt, _ = time.Parse(time.RFC3339Nano, due)
		if !r.DueAt.After(now) {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

func (s *sqliteStore) MarkReverted(intentID string) error {
	_, err := s.db.Exec(`DELETE FROM pending_reverts WHERE intent_id=?`, intentID)
	return err
}

func (s *sqliteStore) Close() error { return s.db.Close() }
