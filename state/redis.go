package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyAudit   = "sloppy:audit"
	keyReverts = "sloppy:pending_reverts"

	// appliedRetention bounds idempotency keys (>> the longest revert TTL).
	appliedRetention = 30 * 24 * time.Hour
	// actionRetention bounds the per-rule firing log (>> the longest budget window).
	actionRetention = 7 * 24 * time.Hour
	// usageRetention bounds per-tenant usage (>> the longest spend window).
	usageRetention = 48 * time.Hour
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

// ClaimIntent uses SET ... NX as the atomic at-most-once gate: the first caller
// for an id sets the key (claimed=true); concurrent callers across replicas get
// a no-op (claimed=false) because NX only writes when the key is absent. The key
// carries a TTL so the idempotency set can't grow forever.
func (s *redisStore) ClaimIntent(ctx context.Context, id string) (bool, error) {
	ok, err := s.c.SetNX(ctx, "sloppy:applied:"+id, 1, appliedRetention).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ReleaseIntent deletes the idempotency key so a failed actuation can be retried.
func (s *redisStore) ReleaseIntent(ctx context.Context, id string) error {
	return s.c.Del(ctx, "sloppy:applied:"+id).Err()
}

type auditRec struct {
	TS       string `json:"ts"`
	Kind     string `json:"kind"`
	Detail   string `json:"detail"`
	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
}

// AppendAudit links a new entry atomically via WATCH/MULTI/EXEC with retry, so
// concurrent appends (across replicas) cannot fork the tamper-evident chain.
func (s *redisStore) AppendAudit(ctx context.Context, kind, detail string) (AuditEntry, error) {
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

func (s *redisStore) Audit(ctx context.Context) ([]AuditEntry, error) {
	vals, err := s.c.LRange(ctx, keyAudit, 0, -1).Result()
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

func (s *redisStore) VerifyAudit(ctx context.Context) bool {
	es, err := s.Audit(ctx)
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

func (s *redisStore) ScheduleRevert(ctx context.Context, r PendingRevert) error {
	b, _ := json.Marshal(revertRec{Kind: r.Kind, Target: r.Target, Args: r.ArgsJSON, DueAt: r.DueAt.UTC().Format(time.RFC3339Nano)})
	return s.c.HSetNX(ctx, keyReverts, r.IntentID, b).Err() // idempotent on IntentID
}

func (s *redisStore) DueReverts(ctx context.Context, now time.Time) ([]PendingRevert, error) {
	m, err := s.c.HGetAll(ctx, keyReverts).Result()
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

func (s *redisStore) MarkReverted(ctx context.Context, id string) error {
	return s.c.HDel(ctx, keyReverts, id).Err()
}

func (s *redisStore) RecordAction(ctx context.Context, ruleSHA string, at time.Time) error {
	key := "sloppy:ract:" + ruleSHA
	ns := at.UnixNano()
	if err := s.c.ZAdd(ctx, key, redis.Z{Score: float64(ns), Member: strconv.FormatInt(ns, 10)}).Err(); err != nil {
		return err
	}
	// Bound the key: prune entries older than retention, then refresh its TTL.
	cutoff := at.Add(-actionRetention).UnixNano()
	_ = s.c.ZRemRangeByScore(ctx, key, "-inf", "("+strconv.FormatInt(cutoff, 10)).Err()
	return s.c.Expire(ctx, key, actionRetention).Err()
}

func (s *redisStore) CountActions(ctx context.Context, ruleSHA string, since time.Time) (int, error) {
	n, err := s.c.ZCount(ctx, "sloppy:ract:"+ruleSHA, strconv.FormatInt(since.UnixNano(), 10), "+inf").Result()
	return int(n), err
}

func (s *redisStore) RecordOutstanding(ctx context.Context, key string, r PendingRevert) error {
	b, _ := json.Marshal(revertRec{Kind: r.Kind, Target: r.Target, Args: r.ArgsJSON})
	return s.c.HSet(ctx, "sloppy:onclear:"+key, r.IntentID, b).Err()
}

func (s *redisStore) Outstanding(ctx context.Context, key string) ([]PendingRevert, error) {
	m, err := s.c.HGetAll(ctx, "sloppy:onclear:"+key).Result()
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

func (s *redisStore) ClearOutstanding(ctx context.Context, key string) error {
	return s.c.Del(ctx, "sloppy:onclear:"+key).Err()
}

func (s *redisStore) RecordUsage(ctx context.Context, tenant, model string, cost float64, at time.Time) error {
	key := "sloppy:usage:" + tenant
	ns := at.UnixNano()
	member := fmt.Sprintf("%d:%g:%s", ns, cost, model) // unique-ish; carries the cost
	if err := s.c.ZAdd(ctx, key, redis.Z{Score: float64(ns), Member: member}).Err(); err != nil {
		return err
	}
	cutoff := at.Add(-usageRetention).UnixNano()
	_ = s.c.ZRemRangeByScore(ctx, key, "-inf", "("+strconv.FormatInt(cutoff, 10)).Err()
	return s.c.Expire(ctx, key, usageRetention).Err()
}

func (s *redisStore) SpendSince(ctx context.Context, tenant string, since time.Time) (float64, error) {
	members, err := s.c.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     "sloppy:usage:" + tenant,
		Start:   strconv.FormatInt(since.UnixNano(), 10),
		Stop:    "+inf",
		ByScore: true,
	}).Result()
	if err != nil {
		return 0, err
	}
	var sum float64
	for _, m := range members {
		parts := strings.SplitN(m, ":", 3)
		if len(parts) < 2 {
			continue
		}
		c, _ := strconv.ParseFloat(parts[1], 64)
		sum += c
	}
	return sum, nil
}

// PruneUsage is a no-op for Redis: usage keys self-prune on write (ZREMRANGEBYSCORE)
// and carry a TTL, so there's no unbounded growth to sweep.
func (s *redisStore) PruneUsage(ctx context.Context, before time.Time) error { return nil }

func (s *redisStore) Close() error { return s.c.Close() }
