package state

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyApplied = "sloppy:applied_intents"
	keyAudit   = "sloppy:audit"
	keyReverts = "sloppy:pending_reverts"
)

type redisStore struct {
	c   *redis.Client
	ctx context.Context
}

// OpenRedis returns a Redis-backed Store (multi-replica capable). Same contract as SQLite.
func OpenRedis(addr string) (Store, error) {
	c := redis.NewClient(&redis.Options{Addr: addr})
	ctx := context.Background()
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &redisStore{c: c, ctx: ctx}, nil
}

func (s *redisStore) IsIntentApplied(id string) (bool, error) {
	return s.c.SIsMember(s.ctx, keyApplied, id).Result()
}

func (s *redisStore) MarkIntentApplied(id string) error {
	return s.c.SAdd(s.ctx, keyApplied, id).Err()
}

type auditRec struct {
	TS       string `json:"ts"`
	Kind     string `json:"kind"`
	Detail   string `json:"detail"`
	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
}

func (s *redisStore) AppendAudit(kind, detail string) (AuditEntry, error) {
	prev := ""
	if last, err := s.c.LIndex(s.ctx, keyAudit, -1).Result(); err == nil && last != "" {
		var lr auditRec
		if json.Unmarshal([]byte(last), &lr) == nil {
			prev = lr.Hash
		}
	}
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	h := ChainHash(ts, kind, detail, prev)
	b, _ := json.Marshal(auditRec{TS: ts, Kind: kind, Detail: detail, PrevHash: prev, Hash: h})
	n, err := s.c.RPush(s.ctx, keyAudit, b).Result()
	if err != nil {
		return AuditEntry{}, err
	}
	t, _ := time.Parse(time.RFC3339Nano, ts)
	return AuditEntry{Seq: int(n), TS: t, Kind: kind, Detail: detail, PrevHash: prev, Hash: h}, nil
}

func (s *redisStore) Audit() ([]AuditEntry, error) {
	vals, err := s.c.LRange(s.ctx, keyAudit, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]AuditEntry, 0, len(vals))
	for i, v := range vals {
		var r auditRec
		if err := json.Unmarshal([]byte(v), &r); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339Nano, r.TS)
		out = append(out, AuditEntry{Seq: i + 1, TS: t, Kind: r.Kind, Detail: r.Detail, PrevHash: r.PrevHash, Hash: r.Hash})
	}
	return out, nil
}

func (s *redisStore) VerifyAudit() bool {
	vals, err := s.c.LRange(s.ctx, keyAudit, 0, -1).Result()
	if err != nil {
		return false
	}
	prev := ""
	for _, v := range vals {
		var r auditRec
		if json.Unmarshal([]byte(v), &r) != nil {
			return false
		}
		if r.PrevHash != prev || r.Hash != ChainHash(r.TS, r.Kind, r.Detail, prev) {
			return false
		}
		prev = r.Hash
	}
	return true
}

type revertRec struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Args   string `json:"args"`
	DueAt  string `json:"due_at"`
}

func (s *redisStore) ScheduleRevert(r PendingRevert) error {
	b, _ := json.Marshal(revertRec{Kind: r.Kind, Target: r.Target, Args: r.ArgsJSON, DueAt: r.DueAt.UTC().Format(time.RFC3339Nano)})
	return s.c.HSetNX(s.ctx, keyReverts, r.IntentID, b).Err() // idempotent on IntentID
}

func (s *redisStore) DueReverts(now time.Time) ([]PendingRevert, error) {
	m, err := s.c.HGetAll(s.ctx, keyReverts).Result()
	if err != nil {
		return nil, err
	}
	var out []PendingRevert
	for id, v := range m {
		var rec revertRec
		if json.Unmarshal([]byte(v), &rec) != nil {
			continue
		}
		due, _ := time.Parse(time.RFC3339Nano, rec.DueAt)
		if !due.After(now) {
			out = append(out, PendingRevert{IntentID: id, Kind: rec.Kind, Target: rec.Target, ArgsJSON: rec.Args, DueAt: due})
		}
	}
	return out, nil
}

func (s *redisStore) MarkReverted(id string) error {
	return s.c.HDel(s.ctx, keyReverts, id).Err()
}

func (s *redisStore) Close() error { return s.c.Close() }
