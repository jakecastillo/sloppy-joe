package state

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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
);
CREATE TABLE IF NOT EXISTS rule_actions (rule_sha TEXT, ts TEXT);
CREATE INDEX IF NOT EXISTS idx_rule_actions ON rule_actions(rule_sha, ts);
CREATE TABLE IF NOT EXISTS onclear (
  key TEXT, intent_id TEXT, kind TEXT, target TEXT, args TEXT,
  PRIMARY KEY(key, intent_id)
);
CREATE TABLE IF NOT EXISTS usage (tenant TEXT, model TEXT, cost REAL, ts TEXT);
CREATE INDEX IF NOT EXISTS idx_usage ON usage(tenant, ts);`

// OpenSQLite opens (and migrates) a SQLite-backed Store. Pure-Go driver (no cgo).
//
// SQLite allows a single writer; the daemon serves requests concurrently, so we
// serialize writers (SetMaxOpenConns(1)) and set busy_timeout + WAL.
func OpenSQLite(path string) (Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}

// ClaimIntent inserts the idempotency key in one statement. A plain INSERT (not
// OR IGNORE) lets us distinguish the winner from a loser: the winner affects one
// row; a duplicate trips the PRIMARY KEY constraint, which we treat as a clean
// "someone else already claimed it" (false, nil) rather than an I/O error. With
// SetMaxOpenConns(1) writes serialize, so concurrent claims of the same id can
// never both succeed.
func (s *sqliteStore) ClaimIntent(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO applied_intents(intent_id) VALUES(?)`, id)
	if err != nil {
		if isUniqueViolation(err) {
			return false, nil // already claimed by another caller — not an error
		}
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// isUniqueViolation reports whether err is a SQLite PRIMARY KEY / UNIQUE
// constraint failure (extended codes share the SQLITE_CONSTRAINT primary code).
func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}

// ReleaseIntent deletes the idempotency key so a failed actuation can be retried.
func (s *sqliteStore) ReleaseIntent(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM applied_intents WHERE intent_id=?`, id)
	return err
}

// AppendAudit is transactional: read-prev-hash + insert is one atomic unit so
// concurrent appends can't fork the chain (with SetMaxOpenConns(1)).
func (s *sqliteStore) AppendAudit(ctx context.Context, kind, detail string) (AuditEntry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuditEntry{}, err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back unless committed

	var prev string
	_ = tx.QueryRowContext(ctx, `SELECT hash FROM audit ORDER BY seq DESC LIMIT 1`).Scan(&prev)
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	h := ChainHash(ts, kind, detail, prev)
	res, err := tx.ExecContext(ctx, `INSERT INTO audit(ts,kind,detail,prev_hash,hash) VALUES(?,?,?,?,?)`, ts, kind, detail, prev, h)
	if err != nil {
		return AuditEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuditEntry{}, err
	}
	id, _ := res.LastInsertId()
	t, _ := time.Parse(time.RFC3339Nano, ts)
	return AuditEntry{Seq: int(id), TS: t, Kind: kind, Detail: detail, PrevHash: prev, Hash: h}, nil
}

func (s *sqliteStore) Audit(ctx context.Context) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq,ts,kind,detail,prev_hash,hash FROM audit ORDER BY seq ASC`)
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

func (s *sqliteStore) VerifyAudit(ctx context.Context) bool {
	es, err := s.Audit(ctx)
	if err != nil {
		return false
	}
	return VerifyChain(es)
}

func (s *sqliteStore) ScheduleRevert(ctx context.Context, r PendingRevert) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO pending_reverts(intent_id,kind,target,args,due_at) VALUES(?,?,?,?,?)`,
		r.IntentID, r.Kind, r.Target, r.ArgsJSON, r.DueAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteStore) DueReverts(ctx context.Context, now time.Time) ([]PendingRevert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT intent_id,kind,target,args,due_at FROM pending_reverts ORDER BY due_at ASC`)
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

func (s *sqliteStore) MarkReverted(ctx context.Context, intentID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_reverts WHERE intent_id=?`, intentID)
	return err
}

func (s *sqliteStore) RecordAction(ctx context.Context, ruleSHA string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO rule_actions(rule_sha,ts) VALUES(?,?)`, ruleSHA, at.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteStore) CountActions(ctx context.Context, ruleSHA string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rule_actions WHERE rule_sha=? AND ts>=?`,
		ruleSHA, since.UTC().Format(time.RFC3339Nano)).Scan(&n)
	return n, err
}

func (s *sqliteStore) RecordOutstanding(ctx context.Context, key string, r PendingRevert) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO onclear(key,intent_id,kind,target,args) VALUES(?,?,?,?,?)`,
		key, r.IntentID, r.Kind, r.Target, r.ArgsJSON)
	return err
}

func (s *sqliteStore) Outstanding(ctx context.Context, key string) ([]PendingRevert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT intent_id,kind,target,args FROM onclear WHERE key=?`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRevert
	for rows.Next() {
		var r PendingRevert
		if err := rows.Scan(&r.IntentID, &r.Kind, &r.Target, &r.ArgsJSON); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sqliteStore) ClearOutstanding(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM onclear WHERE key=?`, key)
	return err
}

func (s *sqliteStore) RecordUsage(ctx context.Context, tenant, model string, cost float64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO usage(tenant,model,cost,ts) VALUES(?,?,?,?)`,
		tenant, model, cost, at.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteStore) SpendSince(ctx context.Context, tenant string, since time.Time) (float64, error) {
	var sum float64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost),0) FROM usage WHERE tenant=? AND ts>=?`,
		tenant, since.UTC().Format(time.RFC3339Nano)).Scan(&sum)
	return sum, err
}

func (s *sqliteStore) PruneUsage(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM usage WHERE ts<?`, before.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteStore) Close() error { return s.db.Close() }
