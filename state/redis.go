package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyApplied = "sloppy:applied_intents"
	keyAudit   = "sloppy:audit"
	keyReverts = "sloppy:pending_reverts"
)

type redisStore struct {
	c *redis.Client
}

// OpenRedis returns a Redis-backed Store (multi-replica capable). Same contract as SQLite.
func OpenRedis(addr string) (Store, error) {
	c := redis.NewClient(&redis.Options{Addr: addr})
	if err := c.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &redisStore{c: c}, nil
}

func (s *redisStore) IsIntentApplied(id string) (bool, error) {
	return s.c.SIsMember(context.Background(), keyApplied, id).Result()
}

func (s *redisStore) MarkIntentApplied(id string) error {
	return s.c.SAdd(context.Background(), keyApplied, id).Err()
}

type auditRec struct {
	TS       string `json:"ts"`
	Kind     string `json:"kind"`
	Detail   string `json:"detail"`
	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
}

// AppendAudit links a new entry atomically. The read-prev + push is wrapped in
// WATCH/MULTI/EXEC with retry-on-conflict so concurrent appends (across replicas)
// cannot fork the tamper-evident chain.
func (s *redisStore) AppendAudit(kind, detail string) (AuditEntry, error) {
	ctx := context.Background()
	const maxRetries = 16
	var entry AuditEntry
	txf := func(tx *redis.Tx) error {
		prev := ""
		if last, err := tx.LIndex(ctx, keyAudit, -1).Result(); err == nil && last != "" {
			var lr auditRec
			if json.Unmarshal([]byte(last), &lr) == nil {
				prev = lr.Hash
			}
		}
		ts := time.Now().UTC().Format(time.RFC3339Nano)
		h := ChainHash(ts, kind, detail, prev)
		b, _ := json.Marshal(auditRec{TS: ts, Kind: kind, Detail: detail, PrevHash: prev, Hash: h})
		var rp *redis.IntCmd
		if _, err := tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			rp = p.RPush(ctx, keyAudit, b)
			return nil
		}); err != nil {
			return err
		}
		t, _ := time.Parse(time.RFC3339Nano, ts)
		entry = AuditEntry{Seq: int(rp.Val()), TS: t, Kind: kind, Detail: detail, PrevHash: prev, Hash: h}
		return nil
	}
	for i := 0; i < maxRetries; i++ {
		err := s.c.Watch(ctx, txf, keyAudit)
		if err == nil {
			return entry, nil
		}
		if err == redis.TxFailedErr {
			continue // optimistic-lock conflict — retry
		}
		return AuditEntry{}, err
	}
	return AuditEntry{}, fmt.Errorf("redis: audit append contention after %d retries", maxRetries)
}

func (s *redisStore) Audit() ([]AuditEntry, error) {
	vals, err := s.c.LRange(context.Background(), keyAudit, 0, -1).Result()
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
	es, err := s.Audit()
	if err != nil {
		return false
	}
	return VerifyChain(es)
}

type revertRec struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Args   string `json:"args"`
	DueAt  string `json:"due_at"`
}

func (s *redisStore) ScheduleRevert(r PendingRevert) error {
	b, _ := json.Marshal(revertRec{Kind: r.Kind, Target: r.Target, Args: r.ArgsJSON, DueAt: r.DueAt.UTC().Format(time.RFC3339Nano)})
	return s.c.HSetNX(context.Background(), keyReverts, r.IntentID, b).Err() // idempotent on IntentID
}

func (s *redisStore) DueReverts(now time.Time) ([]PendingRevert, error) {
	m, err := s.c.HGetAll(context.Background(), keyReverts).Result()
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
	return s.c.HDel(context.Background(), keyReverts, id).Err()
}

func (s *redisStore) RecordAction(ruleSHA string, at time.Time) error {
	ns := at.UnixNano()
	return s.c.ZAdd(context.Background(), "sloppy:ract:"+ruleSHA,
		redis.Z{Score: float64(ns), Member: strconv.FormatInt(ns, 10)}).Err()
}

func (s *redisStore) CountActions(ruleSHA string, since time.Time) (int, error) {
	n, err := s.c.ZCount(context.Background(), "sloppy:ract:"+ruleSHA,
		strconv.FormatInt(since.UnixNano(), 10), "+inf").Result()
	return int(n), err
}

func (s *redisStore) RecordOutstanding(key string, r PendingRevert) error {
	b, _ := json.Marshal(revertRec{Kind: r.Kind, Target: r.Target, Args: r.ArgsJSON})
	return s.c.HSet(context.Background(), "sloppy:onclear:"+key, r.IntentID, b).Err()
}

func (s *redisStore) Outstanding(key string) ([]PendingRevert, error) {
	m, err := s.c.HGetAll(context.Background(), "sloppy:onclear:"+key).Result()
	if err != nil {
		return nil, err
	}
	var out []PendingRevert
	for id, v := range m {
		var rec revertRec
		if json.Unmarshal([]byte(v), &rec) != nil {
			continue
		}
		out = append(out, PendingRevert{IntentID: id, Kind: rec.Kind, Target: rec.Target, ArgsJSON: rec.Args})
	}
	return out, nil
}

func (s *redisStore) ClearOutstanding(key string) error {
	return s.c.Del(context.Background(), "sloppy:onclear:"+key).Err()
}

func (s *redisStore) Close() error { return s.c.Close() }
