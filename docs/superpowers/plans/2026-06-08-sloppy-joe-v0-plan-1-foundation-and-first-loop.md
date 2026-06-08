# Sloppy Joe v0 — Plan 1: Foundation + First Closed Loop

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Sloppy Joe Go project and close the observe→decide→act loop end-to-end for one signal: a `cost.budget_burn` Signal evaluated by a YAML+CEL Rule fires a signed, reversible `route_override` Intent against LiteLLM (mocked in tests), opens a GitHub issue, pages Slack, and records a hash-chained audit receipt — surviving a restart without double-firing.

**Architecture:** Library-first Go (`libsloppyjoe`) with a thin CLI. Off the inference hot path. Layers: `core` (types) → `secrets` (broker) → `state` (SQLite store + hash-chained audit) → `rules` (YAML loader + CEL eval + level-triggered reconciler) → `intent` (build + ed25519 sign) → `actuator` (interface + litellm/github/slack adapters) → `engine` (wires it together) → `cmd/sloppy` (CLI). Cost is carried *in the signal* for Plan 1; the derived Cost Ledger and OTLP ingest come in Plan 2.

**Tech Stack:** Go 1.23 · `github.com/google/cel-go` (rules) · `modernc.org/sqlite` (pure-Go, CGO-free SQLite → static binary) · `gopkg.in/yaml.v3` · `crypto/ed25519` (stdlib signing; Sigstore deferred) · `net/http` + `net/http/httptest` (actuators + tests).

**Module path (provisional):** `github.com/sloppyjoe/sloppy` — change later with `go mod edit -module` once the public handle is finalized.

---

## File structure (created by this plan)

```
sloppy-joe/
├── go.mod · go.sum · Makefile
├── version.go                       package sloppyjoe — Version const
├── version_test.go
├── core/
│   ├── signal.go                    Signal, Subject, Evidence, Severity
│   ├── signal_test.go
│   ├── intent.go                    RemediationIntent, Action kinds
│   ├── receipt.go                   Receipt, Outcome
│   └── ids.go                       deterministic-friendly id helpers
├── secrets/
│   ├── broker.go                    Broker interface + envBroker (JIT, zeroize)
│   └── broker_test.go
├── state/
│   ├── store.go                     Store interface
│   ├── sqlite.go                    sqliteStore (incidents, applied-intents)
│   ├── sqlite_test.go
│   ├── audit.go                     AuditLog hash-chain (append + verify)
│   └── audit_test.go
├── rules/
│   ├── rule.go                      Rule struct (parsed)
│   ├── parse.go                     YAML → []Rule
│   ├── parse_test.go
│   ├── cel.go                       CEL env + compiled condition
│   ├── cel_test.go
│   ├── reconcile.go                 Reconciler: (signal,state,rules)→[]Intent
│   └── reconcile_test.go
├── intent/
│   ├── build.go                     BuildIntents from rule Actions
│   ├── sign.go                      ed25519 Signer/Verify
│   └── sign_test.go
├── actuator/
│   ├── actuator.go                  Actuator interface + Registry + fake
│   ├── actuator_test.go
│   ├── litellm.go                   route_override (Apply/Revert)
│   ├── litellm_test.go
│   ├── github.go                    open_issue
│   ├── github_test.go
│   ├── slack.go                     notify/page
│   └── slack_test.go
├── engine/
│   ├── engine.go                    Engine.Handle(signal): reconcile→govern→actuate→audit
│   └── engine_test.go               full closed-loop test with fakes
└── cmd/sloppy/
    └── main.go                      CLI: up · rules apply · inject · audit tail
```

---

## Task 1: Project scaffold + toolchain smoke test

**Files:**
- Create: `go.mod`, `Makefile`, `version.go`, `version_test.go`

- [ ] **Step 1: Write the failing test**

`version_test.go`:
```go
package sloppyjoe

import "testing"

func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}
```

- [ ] **Step 2: Initialize the module and verify the test fails to build**

Run:
```bash
cd /Users/jakecastillo/Documents/GitHub/sloppy-joe
go mod init github.com/sloppyjoe/sloppy
go test ./...
```
Expected: FAIL — `undefined: Version`.

- [ ] **Step 3: Write minimal implementation**

`version.go`:
```go
// Package sloppyjoe is the library-first core of Sloppy Joe, an AI-ops control loop.
package sloppyjoe

// Version is the current build version.
const Version = "0.0.0-dev"
```

`Makefile`:
```make
.PHONY: test build tidy
test:
	go test ./...
build:
	CGO_ENABLED=0 go build -o bin/sloppy ./cmd/sloppy
tidy:
	go mod tidy
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS (`ok github.com/sloppyjoe/sloppy`).

- [ ] **Step 5: Commit**

```bash
git add go.mod version.go version_test.go Makefile
git commit -m "chore: scaffold Go module + toolchain smoke test"
```

---

## Task 2: Core Signal types

**Files:**
- Create: `core/signal.go`, `core/signal_test.go`

- [ ] **Step 1: Write the failing test**

`core/signal_test.go`:
```go
package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSignalJSONRoundTrip(t *testing.T) {
	s := Signal{
		ID:             "evt-1",
		Source:         "litellm/otel",
		Type:           "cost.budget_burn",
		Time:           time.Unix(1749340800, 0).UTC(),
		Subject:        Subject{Tenant: "acme", Alias: "gpt-4o"},
		Severity:       SeverityCritical,
		CorrelationKey: "acme:cost",
		DedupWindow:    5 * time.Minute,
		Data:           map[string]any{"spend_1h_usd": 7.5},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Signal
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != "cost.budget_burn" || got.Subject.Tenant != "acme" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Data["spend_1h_usd"].(float64) != 7.5 {
		t.Fatalf("data lost: %+v", got.Data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/ -run TestSignalJSONRoundTrip -v`
Expected: FAIL — `undefined: Signal`.

- [ ] **Step 3: Write minimal implementation**

`core/signal.go`:
```go
// Package core holds Sloppy Joe's shared vocabulary types.
package core

import "time"

// Severity classifies a Signal's operational urgency.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Subject identifies what a Signal is about.
type Subject struct {
	Tenant     string `json:"tenant,omitempty"`
	Deployment string `json:"deployment,omitempty"`
	Alias      string `json:"alias,omitempty"`
}

// Evidence is a pointer to supporting data (otel span link, metric snapshot).
type Evidence struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// Signal is an OTel-GenAI event wrapped in a CloudEvents-shaped envelope.
type Signal struct {
	ID             string         `json:"id"`
	Source         string         `json:"source"`
	Type           string         `json:"type"`
	Time           time.Time      `json:"time"`
	Subject        Subject        `json:"subject"`
	Severity       Severity       `json:"severity"`
	CorrelationKey string         `json:"correlation_key"`
	DedupWindow    time.Duration  `json:"dedup_window"`
	Evidence       []Evidence     `json:"evidence,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/ -run TestSignalJSONRoundTrip -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/signal.go core/signal_test.go
git commit -m "feat(core): add Signal envelope type"
```

---

## Task 3: Core Intent + Receipt + id helper

**Files:**
- Create: `core/intent.go`, `core/receipt.go`, `core/ids.go`

- [ ] **Step 1: Write the failing test**

Append to `core/signal_test.go` (same package) a new file `core/intent_test.go`:
```go
package core

import "testing"

func TestIntentCanonicalBytesStable(t *testing.T) {
	i := RemediationIntent{
		ID:      "int-1",
		Kind:    ActionRouteOverride,
		Target:  "gpt-4o",
		Args:    map[string]any{"to": "ollama/llama3", "ttl": "30m"},
		RuleSHA: "abc123",
	}
	a := i.CanonicalBytes()
	b := i.CanonicalBytes()
	if string(a) != string(b) {
		t.Fatal("CanonicalBytes must be deterministic for signing")
	}
	if len(a) == 0 {
		t.Fatal("CanonicalBytes empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/ -run TestIntentCanonicalBytesStable -v`
Expected: FAIL — `undefined: RemediationIntent`.

- [ ] **Step 3: Write minimal implementation**

`core/intent.go`:
```go
package core

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// ActionKind enumerates the governed actions a rule may request.
type ActionKind string

const (
	ActionRouteOverride ActionKind = "route_override"
	ActionOpenIssue     ActionKind = "open_issue"
	ActionPage          ActionKind = "page"
)

// RemediationIntent is a signed, reversible request to change the world.
type RemediationIntent struct {
	ID        string         `json:"id"`
	Kind      ActionKind     `json:"kind"`
	Target    string         `json:"target"`
	Args      map[string]any `json:"args,omitempty"`
	TTL       time.Duration  `json:"ttl,omitempty"`
	Evidence  []Evidence     `json:"evidence,omitempty"`
	RuleSHA   string         `json:"rule_sha"`
	Signature string         `json:"signature,omitempty"`
}

// CanonicalBytes returns a deterministic serialization (sorted arg keys,
// signature excluded) suitable for signing and idempotency keys.
func (i RemediationIntent) CanonicalBytes() []byte {
	var sb strings.Builder
	sb.WriteString(i.ID + "|" + string(i.Kind) + "|" + i.Target + "|" + i.RuleSHA + "|" + i.TTL.String() + "|")
	keys := make([]string, 0, len(i.Args))
	for k := range i.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := json.Marshal(i.Args[k])
		sb.WriteString(k + "=" + string(v) + ";")
	}
	return []byte(sb.String())
}
```

`core/receipt.go`:
```go
package core

import "time"

// Outcome records the result of applying an intent.
type Outcome string

const (
	OutcomeApplied  Outcome = "applied"
	OutcomeReverted Outcome = "reverted"
	OutcomeFailed   Outcome = "failed"
)

// Receipt is the audited proof of an actuator action.
type Receipt struct {
	IntentID  string    `json:"intent_id"`
	Actuator  string    `json:"actuator"`
	AppliedAt time.Time `json:"applied_at"`
	Before    any       `json:"before,omitempty"`
	After     any       `json:"after,omitempty"`
	Outcome   Outcome   `json:"outcome"`
	Signature string    `json:"signature,omitempty"`
}
```

`core/ids.go`:
```go
package core

import (
	"crypto/sha256"
	"encoding/hex"
)

// DeterministicID derives a stable id from parts (used for idempotency keys).
func DeterministicID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/ -v`
Expected: PASS (all core tests).

- [ ] **Step 5: Commit**

```bash
git add core/intent.go core/receipt.go core/ids.go core/intent_test.go
git commit -m "feat(core): add Intent (canonical-signable) and Receipt types"
```

---

## Task 4: ed25519 signing

**Files:**
- Create: `intent/sign.go`, `intent/sign_test.go`

- [ ] **Step 1: Write the failing test**

`intent/sign_test.go`:
```go
package intent

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestSignAndVerify(t *testing.T) {
	s, err := NewEd25519Signer()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	i := core.RemediationIntent{ID: "int-1", Kind: core.ActionRouteOverride, Target: "gpt-4o", RuleSHA: "sha"}
	sig := s.Sign(i.CanonicalBytes())
	if sig == "" {
		t.Fatal("empty signature")
	}
	if !s.Verify(i.CanonicalBytes(), sig) {
		t.Fatal("valid signature failed to verify")
	}
	if s.Verify([]byte("tampered"), sig) {
		t.Fatal("tampered payload verified — must fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./intent/ -run TestSignAndVerify -v`
Expected: FAIL — `undefined: NewEd25519Signer`.

- [ ] **Step 3: Write minimal implementation**

`intent/sign.go`:
```go
// Package intent builds and signs RemediationIntents.
package intent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
)

// Signer signs and verifies canonical intent bytes.
type Signer interface {
	Sign(payload []byte) string
	Verify(payload []byte, sig string) bool
}

type ed25519Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// NewEd25519Signer generates an ephemeral keypair (v0; persisted keys + Sigstore later).
func NewEd25519Signer() (Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &ed25519Signer{priv: priv, pub: pub}, nil
}

func (s *ed25519Signer) Sign(payload []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, payload))
}

func (s *ed25519Signer) Verify(payload []byte, sig string) bool {
	raw, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(s.pub, payload, raw)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./intent/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add intent/sign.go intent/sign_test.go
git commit -m "feat(intent): ed25519 signer for intents/receipts"
```

---

## Task 5: Secret broker (env-based)

**Files:**
- Create: `secrets/broker.go`, `secrets/broker_test.go`

- [ ] **Step 1: Write the failing test**

`secrets/broker_test.go`:
```go
package secrets

import "testing"

func TestEnvBrokerGetAndDeny(t *testing.T) {
	t.Setenv("SLOPPY_TOKEN_LITELLM", "admin-xyz")
	b := NewEnvBroker([]string{"litellm"})
	tok, err := b.Get("litellm")
	if err != nil || tok != "admin-xyz" {
		t.Fatalf("expected admin-xyz, got %q err=%v", tok, err)
	}
	if _, err := b.Get("github"); err == nil {
		t.Fatal("expected default-deny for unregistered capability")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./secrets/ -run TestEnvBrokerGetAndDeny -v`
Expected: FAIL — `undefined: NewEnvBroker`.

- [ ] **Step 3: Write minimal implementation**

`secrets/broker.go`:
```go
// Package secrets holds the minimal-surface token broker. Provider keys never
// live here — only scoped admin/notify tokens. In-proc for v0, sidecar-ready.
package secrets

import (
	"fmt"
	"os"
	"strings"
)

// Broker hands out scoped tokens just-in-time. Default-deny by allowlist.
type Broker interface {
	Get(capability string) (string, error)
}

type envBroker struct{ allowed map[string]bool }

// NewEnvBroker reads tokens from SLOPPY_TOKEN_<CAP> env vars, allowlisted by capability.
func NewEnvBroker(allowed []string) Broker {
	m := map[string]bool{}
	for _, a := range allowed {
		m[a] = true
	}
	return &envBroker{allowed: m}
}

func (b *envBroker) Get(capability string) (string, error) {
	if !b.allowed[capability] {
		return "", fmt.Errorf("secrets: capability %q not allowed (default-deny)", capability)
	}
	v := os.Getenv("SLOPPY_TOKEN_" + strings.ToUpper(capability))
	if v == "" {
		return "", fmt.Errorf("secrets: no token for %q", capability)
	}
	return v, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./secrets/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add secrets/broker.go secrets/broker_test.go
git commit -m "feat(secrets): env-backed default-deny token broker"
```

---

## Task 6: Hash-chained audit log

**Files:**
- Create: `state/audit.go`, `state/audit_test.go`

- [ ] **Step 1: Write the failing test**

`state/audit_test.go`:
```go
package state

import "testing"

func TestAuditChainAppendAndVerify(t *testing.T) {
	a := NewMemAudit()
	a.Append(AuditEntry{Kind: "intent.applied", Detail: "reroute acme"})
	a.Append(AuditEntry{Kind: "intent.reverted", Detail: "restore acme"})
	entries := a.All()
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].Hash == "" || entries[1].PrevHash != entries[0].Hash {
		t.Fatal("chain links broken")
	}
	if !a.Verify() {
		t.Fatal("untampered chain failed verify")
	}
	// Tamper:
	entries[0].Detail = "evil"
	a.replaceForTest(entries)
	if a.Verify() {
		t.Fatal("tampered chain must fail verify")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./state/ -run TestAuditChainAppendAndVerify -v`
Expected: FAIL — `undefined: NewMemAudit`.

- [ ] **Step 3: Write minimal implementation**

`state/audit.go`:
```go
// Package state holds Sloppy Joe's durable control-plane state.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// AuditEntry is one record in the tamper-evident audit chain.
type AuditEntry struct {
	Seq      int       `json:"seq"`
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Detail   string    `json:"detail"`
	PrevHash string    `json:"prev_hash"`
	Hash     string    `json:"hash"`
}

func (e AuditEntry) computeHash() string {
	h := sha256.New()
	h.Write([]byte(e.Kind))
	h.Write([]byte(e.Detail))
	h.Write([]byte(e.PrevHash))
	return hex.EncodeToString(h.Sum(nil))
}

// MemAudit is an in-memory hash-chained log (SQLite-backed variant in Task 7).
type MemAudit struct{ entries []AuditEntry }

// NewMemAudit creates an empty chain.
func NewMemAudit() *MemAudit { return &MemAudit{} }

// Append links a new entry to the chain.
func (a *MemAudit) Append(e AuditEntry) AuditEntry {
	e.Seq = len(a.entries)
	e.Time = time.Time{} // deterministic in tests; engine sets real time via WithTime
	if len(a.entries) > 0 {
		e.PrevHash = a.entries[len(a.entries)-1].Hash
	}
	e.Hash = e.computeHash()
	a.entries = append(a.entries, e)
	return e
}

// All returns a copy-safe slice of entries.
func (a *MemAudit) All() []AuditEntry { return a.entries }

// Verify recomputes the chain and checks every link.
func (a *MemAudit) Verify() bool {
	prev := ""
	for _, e := range a.entries {
		if e.PrevHash != prev || e.Hash != e.computeHash() {
			return false
		}
		prev = e.Hash
	}
	return true
}

func (a *MemAudit) replaceForTest(entries []AuditEntry) { a.entries = entries }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state/ -run TestAuditChainAppendAndVerify -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add state/audit.go state/audit_test.go
git commit -m "feat(state): tamper-evident hash-chained audit log"
```

---

## Task 7: SQLite store (incidents + applied-intent idempotency + audit persistence)

**Files:**
- Create: `state/store.go`, `state/sqlite.go`, `state/sqlite_test.go`

- [ ] **Step 1: Write the failing test**

`state/sqlite_test.go`:
```go
package state

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStoreIdempotencyAndAudit(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenSQLite(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// First application of an intent id is "new"; second is a replay.
	if applied, _ := s.IsIntentApplied("int-1"); applied {
		t.Fatal("int-1 should be new")
	}
	if err := s.MarkIntentApplied("int-1"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if applied, _ := s.IsIntentApplied("int-1"); !applied {
		t.Fatal("int-1 should be applied after mark (idempotency key)")
	}

	// Audit persists and verifies across reopen.
	if _, err := s.AppendAudit(AuditEntry{Kind: "intent.applied", Detail: "reroute"}); err != nil {
		t.Fatalf("audit: %v", err)
	}
	s.Close()
	s2, err := OpenSQLite(db)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if !s2.VerifyAudit() {
		t.Fatal("persisted audit chain failed verify after reopen")
	}
}
```

- [ ] **Step 2: Add the dependency and verify the test fails**

Run:
```bash
go get modernc.org/sqlite@latest
go test ./state/ -run TestSQLiteStoreIdempotencyAndAudit -v
```
Expected: FAIL — `undefined: OpenSQLite`.

- [ ] **Step 3: Write minimal implementation**

`state/store.go`:
```go
package state

// Store is the durable backend. SQLite for v0; Redis/Postgres later behind this same interface.
type Store interface {
	IsIntentApplied(intentID string) (bool, error)
	MarkIntentApplied(intentID string) error
	AppendAudit(e AuditEntry) (AuditEntry, error)
	VerifyAudit() bool
	Close() error
}
```

`state/sqlite.go`:
```go
package state

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"

	_ "modernc.org/sqlite"
)

type sqliteStore struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS applied_intents (intent_id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS audit (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT, detail TEXT, prev_hash TEXT, hash TEXT
);`

// OpenSQLite opens (and migrates) a SQLite-backed Store.
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

func hashAudit(kind, detail, prev string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte(detail))
	h.Write([]byte(prev))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *sqliteStore) AppendAudit(e AuditEntry) (AuditEntry, error) {
	var prev string
	_ = s.db.QueryRow(`SELECT hash FROM audit ORDER BY seq DESC LIMIT 1`).Scan(&prev)
	e.PrevHash = prev
	e.Hash = hashAudit(e.Kind, e.Detail, prev)
	res, err := s.db.Exec(`INSERT INTO audit(kind,detail,prev_hash,hash) VALUES(?,?,?,?)`, e.Kind, e.Detail, e.PrevHash, e.Hash)
	if err != nil {
		return e, err
	}
	id, _ := res.LastInsertId()
	e.Seq = int(id)
	return e, nil
}

func (s *sqliteStore) VerifyAudit() bool {
	rows, err := s.db.Query(`SELECT kind,detail,prev_hash,hash FROM audit ORDER BY seq ASC`)
	if err != nil {
		return false
	}
	defer rows.Close()
	prev := ""
	for rows.Next() {
		var kind, detail, ph, h string
		if err := rows.Scan(&kind, &detail, &ph, &h); err != nil {
			return false
		}
		if ph != prev || h != hashAudit(kind, detail, prev) {
			return false
		}
		prev = h
	}
	return true
}

func (s *sqliteStore) Close() error { return s.db.Close() }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./state/ -v && go mod tidy`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add state/store.go state/sqlite.go state/sqlite_test.go go.mod go.sum
git commit -m "feat(state): SQLite store with intent idempotency + persisted audit chain"
```

---

## Task 8: Rule struct + YAML parser

**Files:**
- Create: `rules/rule.go`, `rules/parse.go`, `rules/parse_test.go`

- [ ] **Step 1: Write the failing test**

`rules/parse_test.go`:
```go
package rules

import "testing"

const sampleRule = `
on: cost.budget_burn
when: signal.tenant == "acme" && signal.data.spend_1h_usd > 5.0
for: 5m
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
  - open_issue: { repo: acme/ops }
  - page: { slack: "#oncall" }
with: { dry_run: false, intent_budget: "3/h" }
`

func TestParseRule(t *testing.T) {
	rs, err := ParseRules([]byte(sampleRule))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rs))
	}
	r := rs[0]
	if r.On != "cost.budget_burn" || r.For.String() != "5m0s" {
		t.Fatalf("bad header: %+v", r)
	}
	if len(r.Then) != 3 || r.Then[0].Kind != "route_override" {
		t.Fatalf("bad actions: %+v", r.Then)
	}
	if r.Then[0].Args["to"] != "ollama/llama3" {
		t.Fatalf("bad arg: %+v", r.Then[0].Args)
	}
	if r.SHA == "" {
		t.Fatal("rule SHA must be computed for provenance")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./rules/ -run TestParseRule -v`
Expected: FAIL — `undefined: ParseRules`.

- [ ] **Step 3: Write minimal implementation**

`rules/rule.go`:
```go
// Package rules loads, compiles, and evaluates Sloppy Joe rules.
package rules

import "time"

// Action is one entry in a rule's `then:` list.
type Action struct {
	Kind string
	Args map[string]any
}

// With holds governance knobs.
type With struct {
	DryRun       bool   `yaml:"dry_run"`
	IntentBudget string `yaml:"intent_budget"`
	Rollback     string `yaml:"rollback"`
}

// Rule is a parsed, Git-versioned governance rule.
type Rule struct {
	On    string
	When  string
	For   time.Duration
	Then  []Action
	With  With
	SHA   string // content hash for audit provenance
}
```

`rules/parse.go`:
```go
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type rawRule struct {
	On   string                   `yaml:"on"`
	When string                   `yaml:"when"`
	For  string                   `yaml:"for"`
	Then []map[string]map[string]any `yaml:"then"`
	With With                     `yaml:"with"`
}

// ParseRules parses one YAML document into Rules (one doc = one rule in v0).
func ParseRules(b []byte) ([]Rule, error) {
	var raw rawRule
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("rules: yaml: %w", err)
	}
	if raw.On == "" {
		return nil, fmt.Errorf("rules: missing `on:`")
	}
	var dur time.Duration
	if raw.For != "" {
		d, err := time.ParseDuration(raw.For)
		if err != nil {
			return nil, fmt.Errorf("rules: bad `for:` %q: %w", raw.For, err)
		}
		dur = d
	}
	var actions []Action
	for _, m := range raw.Then {
		for kind, args := range m {
			actions = append(actions, Action{Kind: kind, Args: args})
		}
	}
	sum := sha256.Sum256(b)
	return []Rule{{
		On: raw.On, When: raw.When, For: dur, Then: actions, With: raw.With,
		SHA: hex.EncodeToString(sum[:])[:12],
	}}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./rules/ -run TestParseRule -v && go mod tidy`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add rules/rule.go rules/parse.go rules/parse_test.go go.mod go.sum
git commit -m "feat(rules): YAML rule parser with content SHA"
```

---

## Task 9: CEL condition compile + evaluate

**Files:**
- Create: `rules/cel.go`, `rules/cel_test.go`

- [ ] **Step 1: Write the failing test**

`rules/cel_test.go`:
```go
package rules

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestCELEvaluate(t *testing.T) {
	prog, err := CompileCondition(`signal.tenant == "acme" && signal.data.spend_1h_usd > 5.0`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	hit := core.Signal{Subject: core.Subject{Tenant: "acme"}, Data: map[string]any{"spend_1h_usd": 7.5}}
	miss := core.Signal{Subject: core.Subject{Tenant: "acme"}, Data: map[string]any{"spend_1h_usd": 1.0}}
	if ok, err := prog.Eval(hit, nil); err != nil || !ok {
		t.Fatalf("expected hit, got ok=%v err=%v", ok, err)
	}
	if ok, _ := prog.Eval(miss, nil); ok {
		t.Fatal("expected miss")
	}
}
```

- [ ] **Step 2: Add cel-go and verify the test fails**

Run:
```bash
go get github.com/google/cel-go@latest
go test ./rules/ -run TestCELEvaluate -v
```
Expected: FAIL — `undefined: CompileCondition`.

- [ ] **Step 3: Write minimal implementation**

`rules/cel.go`:
```go
package rules

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/sloppyjoe/sloppy/core"
)

// Condition is a compiled CEL program over {signal, state}.
type Condition struct{ prog cel.Program }

// celEnv exposes `signal` and `state` as dynamic maps.
func celEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("signal", cel.DynType),
		cel.Variable("state", cel.DynType),
	)
}

// CompileCondition compiles a CEL boolean expression.
func CompileCondition(expr string) (*Condition, error) {
	env, err := celEnv()
	if err != nil {
		return nil, err
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("rules: cel compile: %w", iss.Err())
	}
	prog, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	return &Condition{prog: prog}, nil
}

// Eval evaluates the condition. `signal` is flattened so `signal.tenant`,
// `signal.alias`, `signal.severity`, `signal.data.*` are addressable.
func (c *Condition) Eval(sig core.Signal, state map[string]any) (bool, error) {
	if state == nil {
		state = map[string]any{}
	}
	sigMap := map[string]any{
		"tenant":     sig.Subject.Tenant,
		"deployment": sig.Subject.Deployment,
		"alias":      sig.Subject.Alias,
		"severity":   string(sig.Severity),
		"type":       sig.Type,
		"data":       sig.Data,
	}
	out, _, err := c.prog.Eval(map[string]any{"signal": sigMap, "state": state})
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("rules: condition did not return bool")
	}
	return b, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./rules/ -v && go mod tidy`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add rules/cel.go rules/cel_test.go go.mod go.sum
git commit -m "feat(rules): CEL condition compile/eval over signal+state"
```

---

## Task 10: Reconciler — (signal, state, rules) → intents

**Files:**
- Create: `rules/reconcile.go`, `rules/reconcile_test.go`

- [ ] **Step 1: Write the failing test**

`rules/reconcile_test.go`:
```go
package rules

import (
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestReconcileProducesIntents(t *testing.T) {
	rs, err := ParseRules([]byte(sampleRule))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewReconciler(rs)
	if err != nil {
		t.Fatalf("compile rules: %v", err)
	}
	sig := core.Signal{
		Type:    "cost.budget_burn",
		Subject: core.Subject{Tenant: "acme", Alias: "gpt-4o"},
		Data:    map[string]any{"spend_1h_usd": 7.5},
	}
	intents := rec.Reconcile(sig, nil)
	if len(intents) != 3 {
		t.Fatalf("want 3 intents (reroute, issue, page), got %d", len(intents))
	}
	if intents[0].Kind != core.ActionRouteOverride || intents[0].RuleSHA == "" {
		t.Fatalf("bad intent: %+v", intents[0])
	}
	// Wrong type → no intents.
	if got := rec.Reconcile(core.Signal{Type: "other"}, nil); len(got) != 0 {
		t.Fatalf("non-matching type should yield 0, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./rules/ -run TestReconcileProducesIntents -v`
Expected: FAIL — `undefined: NewReconciler`.

- [ ] **Step 3: Write minimal implementation**

`rules/reconcile.go`:
```go
package rules

import (
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

type compiledRule struct {
	rule Rule
	cond *Condition
}

// Reconciler evaluates compiled rules against a signal.
type Reconciler struct{ rules []compiledRule }

// NewReconciler compiles all rule conditions up front.
func NewReconciler(rs []Rule) (*Reconciler, error) {
	out := make([]compiledRule, 0, len(rs))
	for _, r := range rs {
		c, err := CompileCondition(r.When)
		if err != nil {
			return nil, err
		}
		out = append(out, compiledRule{rule: r, cond: c})
	}
	return &Reconciler{rules: out}, nil
}

// Reconcile returns the intents that fire for this signal+state.
// Level-triggered: callers dedupe by intent id (idempotency). `for:` windowing
// is enforced by the engine; the reconciler is the pure decision function.
func (rc *Reconciler) Reconcile(sig core.Signal, state map[string]any) []core.RemediationIntent {
	var intents []core.RemediationIntent
	for _, cr := range rc.rules {
		if cr.rule.On != sig.Type {
			continue
		}
		ok, err := cr.cond.Eval(sig, state)
		if err != nil || !ok {
			continue
		}
		for _, a := range cr.rule.Then {
			intents = append(intents, actionToIntent(a, cr.rule, sig))
		}
	}
	return intents
}

func actionToIntent(a Action, r Rule, sig core.Signal) core.RemediationIntent {
	target := sig.Subject.Alias
	ttl := time.Duration(0)
	if s, ok := a.Args["ttl"].(string); ok {
		if d, err := time.ParseDuration(s); err == nil {
			ttl = d
		}
	}
	id := core.DeterministicID(string(a.Kind), target, r.SHA, sig.CorrelationKey)
	return core.RemediationIntent{
		ID:      id,
		Kind:    core.ActionKind(a.Kind),
		Target:  target,
		Args:    a.Args,
		TTL:     ttl,
		RuleSHA: r.SHA,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./rules/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add rules/reconcile.go rules/reconcile_test.go
git commit -m "feat(rules): level-triggered reconciler producing intents"
```

---

## Task 11: Actuator interface + registry + fake

**Files:**
- Create: `actuator/actuator.go`, `actuator/actuator_test.go`

- [ ] **Step 1: Write the failing test**

`actuator/actuator_test.go`:
```go
package actuator

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestRegistryDispatchAndCapability(t *testing.T) {
	reg := NewRegistry()
	f := &Fake{}
	reg.Register(f)
	r, err := reg.Apply(context.Background(), core.RemediationIntent{Kind: core.ActionRouteOverride, Target: "gpt-4o"})
	if err != nil || r.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", r, err)
	}
	if f.Applied != 1 {
		t.Fatalf("fake should have applied once, got %d", f.Applied)
	}
	// Unknown kind → graceful error, not panic.
	if _, err := reg.Apply(context.Background(), core.RemediationIntent{Kind: "nope"}); err == nil {
		t.Fatal("expected error for unsupported action kind")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./actuator/ -run TestRegistryDispatchAndCapability -v`
Expected: FAIL — `undefined: NewRegistry`.

- [ ] **Step 3: Write minimal implementation**

`actuator/actuator.go`:
```go
// Package actuator applies RemediationIntents to the outside world.
package actuator

import (
	"context"
	"fmt"

	"github.com/sloppyjoe/sloppy/core"
)

// Actuator executes (and can revert) one or more action kinds.
type Actuator interface {
	Capabilities() []core.ActionKind
	Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error)
	Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error)
}

// Registry routes intents to the actuator that declares their kind.
type Registry struct{ byKind map[core.ActionKind]Actuator }

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{byKind: map[core.ActionKind]Actuator{}} }

// Register wires an actuator's declared capabilities.
func (r *Registry) Register(a Actuator) {
	for _, k := range a.Capabilities() {
		r.byKind[k] = a
	}
}

// Apply dispatches one intent.
func (r *Registry) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	a, ok := r.byKind[i.Kind]
	if !ok {
		return core.Receipt{IntentID: i.ID, Outcome: core.OutcomeFailed}, fmt.Errorf("actuator: no actuator for kind %q", i.Kind)
	}
	return a.Apply(ctx, i)
}

// Revert dispatches a revert.
func (r *Registry) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	a, ok := r.byKind[i.Kind]
	if !ok {
		return core.Receipt{IntentID: i.ID, Outcome: core.OutcomeFailed}, fmt.Errorf("actuator: no actuator for kind %q", i.Kind)
	}
	return a.Revert(ctx, i)
}

// Fake is a test actuator covering all kinds.
type Fake struct{ Applied, Reverted int }

func (f *Fake) Capabilities() []core.ActionKind {
	return []core.ActionKind{core.ActionRouteOverride, core.ActionOpenIssue, core.ActionPage}
}
func (f *Fake) Apply(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	f.Applied++
	return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeApplied}, nil
}
func (f *Fake) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	f.Reverted++
	return core.Receipt{IntentID: i.ID, Actuator: "fake", Outcome: core.OutcomeReverted}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./actuator/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add actuator/actuator.go actuator/actuator_test.go
git commit -m "feat(actuator): Actuator interface, registry, fake"
```

---

## Task 12: LiteLLM route_override actuator (Apply/Revert) against an httptest mock

**Files:**
- Create: `actuator/litellm.go`, `actuator/litellm_test.go`

- [ ] **Step 1: Write the failing test**

`actuator/litellm_test.go`:
```go
package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestLiteLLMRouteOverrideApplyRevert(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	a := NewLiteLLM(srv.URL, func() (string, error) { return "admin-xyz", nil })
	i := core.RemediationIntent{
		ID: "int-1", Kind: core.ActionRouteOverride, Target: "gpt-4o",
		Args: map[string]any{"to": "ollama/llama3"},
	}
	rcpt, err := a.Apply(context.Background(), i)
	if err != nil || rcpt.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", rcpt, err)
	}
	if gotAuth != "Bearer admin-xyz" {
		t.Fatalf("missing/incorrect auth: %q", gotAuth)
	}
	if gotPath == "" || gotBody["model"] != "gpt-4o" || gotBody["to"] != "ollama/llama3" {
		t.Fatalf("bad request path=%q body=%+v", gotPath, gotBody)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./actuator/ -run TestLiteLLMRouteOverrideApplyRevert -v`
Expected: FAIL — `undefined: NewLiteLLM`.

- [ ] **Step 3: Write minimal implementation**

`actuator/litellm.go`:
```go
package actuator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// TokenFunc returns a scoped admin token just-in-time (from the secret broker).
type TokenFunc func() (string, error)

// LiteLLM applies route_override via the LiteLLM admin API.
type LiteLLM struct {
	baseURL string
	token   TokenFunc
	client  *http.Client
}

// NewLiteLLM builds the adapter. baseURL is the LiteLLM admin endpoint.
func NewLiteLLM(baseURL string, token TokenFunc) *LiteLLM {
	return &LiteLLM{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (l *LiteLLM) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionRouteOverride} }

func (l *LiteLLM) post(ctx context.Context, body map[string]any) error {
	tok, err := l.token()
	if err != nil {
		return err
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/model/update", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("litellm: admin returned %d", resp.StatusCode)
	}
	return nil
}

func (l *LiteLLM) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	to, _ := i.Args["to"].(string)
	err := l.post(ctx, map[string]any{"model": i.Target, "to": to})
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "litellm", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "litellm", AppliedAt: time.Now().UTC(),
		Before: i.Target, After: to, Outcome: core.OutcomeApplied}, nil
}

func (l *LiteLLM) Revert(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	err := l.post(ctx, map[string]any{"model": i.Target, "to": i.Target}) // restore self-route
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "litellm", Outcome: core.OutcomeFailed}, err
	}
	return core.Receipt{IntentID: i.ID, Actuator: "litellm", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeReverted}, nil
}
```

> NOTE for implementer: the exact LiteLLM admin path/payload (`/model/update`) is a provisional shape — verify against the running LiteLLM admin API during the Task 16 integration step and adjust the single `post` call if needed. The test pins the contract we expect; if LiteLLM differs, update both together.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./actuator/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add actuator/litellm.go actuator/litellm_test.go
git commit -m "feat(actuator): LiteLLM route_override apply/revert"
```

---

## Task 13: GitHub open_issue + Slack page actuators

**Files:**
- Create: `actuator/github.go`, `actuator/github_test.go`, `actuator/slack.go`, `actuator/slack_test.go`

- [ ] **Step 1: Write the failing tests**

`actuator/github_test.go`:
```go
package actuator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestGitHubOpenIssue(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42}`))
	}))
	defer srv.Close()
	a := NewGitHub(srv.URL, func() (string, error) { return "gh-tok", nil })
	i := core.RemediationIntent{ID: "int-2", Kind: core.ActionOpenIssue, RuleSHA: "sha9",
		Args: map[string]any{"repo": "acme/ops"}}
	r, err := a.Apply(context.Background(), i)
	if err != nil || r.Outcome != core.OutcomeApplied {
		t.Fatalf("apply: %+v err=%v", r, err)
	}
	if body["title"] == nil {
		t.Fatal("issue must have a title")
	}
}
```

`actuator/slack_test.go`:
```go
package actuator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sloppyjoe/sloppy/core"
)

func TestSlackPage(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := NewSlack(srv.URL)
	r, err := a.Apply(context.Background(), core.RemediationIntent{ID: "int-3", Kind: core.ActionPage,
		Args: map[string]any{"slack": "#oncall"}})
	if err != nil || r.Outcome != core.OutcomeApplied || !hit {
		t.Fatalf("slack apply failed: %+v err=%v hit=%v", r, err, hit)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./actuator/ -run 'TestGitHubOpenIssue|TestSlackPage' -v`
Expected: FAIL — `undefined: NewGitHub`, `undefined: NewSlack`.

- [ ] **Step 3: Write minimal implementations**

`actuator/github.go`:
```go
package actuator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// GitHub opens issues via the REST API (baseURL overridable for tests).
type GitHub struct {
	baseURL string
	token   TokenFunc
	client  *http.Client
}

// NewGitHub builds the adapter.
func NewGitHub(baseURL string, token TokenFunc) *GitHub {
	return &GitHub{baseURL: baseURL, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

func (g *GitHub) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionOpenIssue} }

func (g *GitHub) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	repo, _ := i.Args["repo"].(string)
	tok, err := g.token()
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeFailed}, err
	}
	payload := map[string]any{
		"title": fmt.Sprintf("Sloppy Joe: auto-mitigation for %s", i.Target),
		"body":  fmt.Sprintf("Automated remediation fired.\n\nIntent: `%s`\nRule SHA: `%s`", i.ID, i.RuleSHA),
	}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/issues", g.baseURL, repo), bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.client.Do(req)
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeFailed}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeFailed}, fmt.Errorf("github: %d", resp.StatusCode)
	}
	return core.Receipt{IntentID: i.ID, Actuator: "github", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

// Revert is a no-op: an opened issue stays as the incident record.
func (g *GitHub) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "github", Outcome: core.OutcomeReverted}, nil
}
```

`actuator/slack.go`:
```go
package actuator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
)

// Slack posts to an incoming webhook URL.
type Slack struct {
	webhook string
	client  *http.Client
}

// NewSlack builds the adapter from a webhook URL.
func NewSlack(webhook string) *Slack {
	return &Slack{webhook: webhook, client: &http.Client{Timeout: 10 * time.Second}}
}

func (s *Slack) Capabilities() []core.ActionKind { return []core.ActionKind{core.ActionPage} }

func (s *Slack) Apply(ctx context.Context, i core.RemediationIntent) (core.Receipt, error) {
	channel, _ := i.Args["slack"].(string)
	payload := map[string]any{"text": fmt.Sprintf("🥪 Sloppy Joe fired %s on %s (%s)", i.Kind, i.Target, channel)}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.webhook, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeFailed}, fmt.Errorf("slack: %d", resp.StatusCode)
	}
	return core.Receipt{IntentID: i.ID, Actuator: "slack", AppliedAt: time.Now().UTC(), Outcome: core.OutcomeApplied}, nil
}

func (s *Slack) Revert(_ context.Context, i core.RemediationIntent) (core.Receipt, error) {
	return core.Receipt{IntentID: i.ID, Actuator: "slack", Outcome: core.OutcomeReverted}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./actuator/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add actuator/github.go actuator/github_test.go actuator/slack.go actuator/slack_test.go
git commit -m "feat(actuator): github open_issue + slack page adapters"
```

---

## Task 14: Engine — wire the closed loop (govern + idempotency + audit)

**Files:**
- Create: `engine/engine.go`, `engine/engine_test.go`

- [ ] **Step 1: Write the failing test**

`engine/engine_test.go`:
```go
package engine

import (
	"context"
	"testing"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

const rule = `
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then:
  - route_override: { alias: gpt-4o, to: ollama/llama3, ttl: 30m }
with: { dry_run: false }
`

func newEngine(t *testing.T) (*Engine, *actuator.Fake, state.Store) {
	t.Helper()
	rs, err := rules.ParseRules([]byte(rule))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.OpenSQLite(t.TempDir() + "/e.db")
	if err != nil {
		t.Fatal(err)
	}
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	return New(rec, reg, st), f, st
}

func TestEngineClosesLoopAndIsIdempotent(t *testing.T) {
	e, f, st := newEngine(t)
	defer st.Close()
	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}

	n, err := e.Handle(context.Background(), sig)
	if err != nil || n != 1 {
		t.Fatalf("first handle: applied=%d err=%v", n, err)
	}
	// Replay the SAME signal → idempotent, no second application.
	n2, _ := e.Handle(context.Background(), sig)
	if n2 != 0 {
		t.Fatalf("replay should apply 0, applied %d", n2)
	}
	if f.Applied != 1 {
		t.Fatalf("actuator should fire exactly once, fired %d", f.Applied)
	}
	if !st.VerifyAudit() {
		t.Fatal("audit chain invalid")
	}
}

func TestEngineDryRunDoesNotActuate(t *testing.T) {
	rs, _ := rules.ParseRules([]byte(`
on: cost.budget_burn
when: signal.data.spend_1h_usd > 5.0
then: [ { route_override: { alias: gpt-4o, to: ollama/llama3 } } ]
with: { dry_run: true }
`))
	rec, _ := rules.NewReconciler(rs)
	st, _ := state.OpenSQLite(t.TempDir() + "/d.db")
	defer st.Close()
	reg := actuator.NewRegistry()
	f := &actuator.Fake{}
	reg.Register(f)
	e := New(rec, reg, st)
	_, _ = e.Handle(context.Background(), core.Signal{Type: "cost.budget_burn",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}})
	if f.Applied != 0 {
		t.Fatalf("dry_run must not actuate, fired %d", f.Applied)
	}
}
```

> NOTE: `dry_run` must be threaded from the rule's `With` into the intent. Add a `DryRun bool` field to `core.RemediationIntent` in Task 3's file *if not present* — it is referenced here. (Implementer: it is NOT in Task 3; add it now to `core/intent.go` and to `actionToIntent` in `rules/reconcile.go`, carrying `r.With.DryRun`. Update Task 10's reconciler accordingly. This cross-task dependency is intentional and called out so you don't miss it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -v`
Expected: FAIL — `undefined: New` (and a missing `DryRun` field until you add it).

- [ ] **Step 3: Write minimal implementation**

First, add `DryRun` to `core/intent.go` struct (field: `DryRun bool \`json:"dry_run,omitempty"\``) and set it in `rules/reconcile.go` `actionToIntent` via a new param `dryRun bool` passed from `cr.rule.With.DryRun`.

`engine/engine.go`:
```go
// Package engine wires reconcile → govern → actuate → audit.
package engine

import (
	"context"
	"fmt"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

// Engine is the off-hot-path control loop core.
type Engine struct {
	rec   *rules.Reconciler
	reg   *actuator.Registry
	store state.Store
}

// New builds an engine.
func New(rec *rules.Reconciler, reg *actuator.Registry, store state.Store) *Engine {
	return &Engine{rec: rec, reg: reg, store: store}
}

// Handle runs one signal through the loop. Returns the count of intents applied.
func (e *Engine) Handle(ctx context.Context, sig core.Signal) (int, error) {
	intents := e.rec.Reconcile(sig, nil)
	applied := 0
	for _, i := range intents {
		if i.DryRun {
			_, _ = e.store.AppendAudit(state.AuditEntry{Kind: "intent.dry_run", Detail: i.ID})
			continue
		}
		// Idempotency: skip already-applied intents (crash-resume safety).
		if done, _ := e.store.IsIntentApplied(i.ID); done {
			continue
		}
		rcpt, err := e.reg.Apply(ctx, i)
		if err != nil {
			_, _ = e.store.AppendAudit(state.AuditEntry{Kind: "intent.failed", Detail: fmt.Sprintf("%s: %v", i.ID, err)})
			continue
		}
		_ = e.store.MarkIntentApplied(i.ID)
		_, _ = e.store.AppendAudit(state.AuditEntry{Kind: "intent.applied",
			Detail: fmt.Sprintf("%s %s→%v rule=%s", i.Kind, i.Target, i.Args["to"], i.RuleSHA)})
		_ = rcpt
		applied++
	}
	return applied, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS (all packages).

- [ ] **Step 5: Commit**

```bash
git add core/intent.go rules/reconcile.go engine/engine.go engine/engine_test.go
git commit -m "feat(engine): closed loop with dry-run, idempotency, audit"
```

---

## Task 15: CLI — `sloppy` (up · rules apply · inject · audit tail)

**Files:**
- Create: `cmd/sloppy/main.go`, `cmd/sloppy/main_test.go`

- [ ] **Step 1: Write the failing test**

`cmd/sloppy/main_test.go`:
```go
package main

import (
	"bytes"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"version"}, &out)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out.Len() == 0 {
		t.Fatal("version printed nothing")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"frobnicate"}, &out); code == 0 {
		t.Fatal("unknown command should be non-zero exit")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/sloppy/ -v`
Expected: FAIL — `undefined: run`.

- [ ] **Step 3: Write minimal implementation**

`cmd/sloppy/main.go`:
```go
// Command sloppy is the Sloppy Joe CLI.
package main

import (
	"fmt"
	"io"
	"os"

	sloppyjoe "github.com/sloppyjoe/sloppy"
)

func run(args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: sloppy <version|up|rules|inject|audit>")
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(out, "sloppy %s\n", sloppyjoe.Version)
		return 0
	case "up", "rules", "inject", "audit":
		fmt.Fprintf(out, "🥪 sloppy: %q not yet wired in Plan 1 skeleton\n", args[0])
		return 0
	default:
		fmt.Fprintf(out, "unknown command: %s\n", args[0])
		return 2
	}
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
```

> NOTE: The `up`/`rules apply`/`inject`/`audit tail` commands are stubbed here so the binary builds and the skeleton is demoable; they are fully wired to the Engine + SQLite + a real config in **Plan 2** (which also adds OTLP ingest and the cost ledger). Plan 1's proof of the closed loop is the `engine` package test (Task 14) and the integration test (Task 16), not the CLI.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/sloppy/ -v && make build`
Expected: PASS; `bin/sloppy` builds.

- [ ] **Step 5: Commit**

```bash
git add cmd/sloppy/main.go cmd/sloppy/main_test.go
git commit -m "feat(cli): sloppy command skeleton (version + stubs)"
```

---

## Task 16: Integration test — full loop against a mock LiteLLM, with crash-resume

**Files:**
- Create: `engine/integration_test.go`

- [ ] **Step 1: Write the failing test**

`engine/integration_test.go`:
```go
package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/state"
)

func TestClosedLoopAgainstMockLiteLLM_CrashResume(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	rs, _ := rules.ParseRules([]byte(rule)) // reuse Task 14's `rule` const
	rec, _ := rules.NewReconciler(rs)
	reg := actuator.NewRegistry()
	reg.Register(actuator.NewLiteLLM(srv.URL, func() (string, error) { return "admin-xyz", nil }))

	dbpath := t.TempDir() + "/loop.db"
	st1, _ := state.OpenSQLite(dbpath)
	e1 := New(rec, reg, st1)
	sig := core.Signal{Type: "cost.budget_burn", CorrelationKey: "acme:cost",
		Subject: core.Subject{Alias: "gpt-4o"}, Data: map[string]any{"spend_1h_usd": 9.0}}
	if n, err := e1.Handle(context.Background(), sig); err != nil || n != 1 {
		t.Fatalf("first run applied=%d err=%v", n, err)
	}
	st1.Close() // simulate crash

	// Restart against the SAME db, replay the SAME signal.
	st2, _ := state.OpenSQLite(dbpath)
	defer st2.Close()
	e2 := New(rec, reg, st2)
	if n, _ := e2.Handle(context.Background(), sig); n != 0 {
		t.Fatalf("after restart, replay must apply 0, applied %d", n)
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("LiteLLM admin must be called exactly once across crash+replay, got %d", got)
	}
	if !st2.VerifyAudit() {
		t.Fatal("audit chain invalid after resume")
	}
}
```

- [ ] **Step 2: Run test to verify it fails (then passes)**

Run: `go test ./engine/ -run TestClosedLoopAgainstMockLiteLLM_CrashResume -v`
Expected: PASS (all building blocks exist by now). If it FAILS on call-count, the idempotency key in Task 14 isn't persisted before the actuator call survives a crash — the design records *applied* only after success, which for at-least-once means a crash *between* admin-call and mark could double-apply; document this as the known at-least-once boundary and assert "exactly once" only for the clean-restart path (which this test exercises).

- [ ] **Step 3: (If needed) tighten ordering**

No code change expected if Task 14 marks-applied after a successful apply and checks before. Confirm the assertion documents the at-least-once boundary in a comment.

- [ ] **Step 4: Run the whole suite**

Run: `go test ./... && make build`
Expected: PASS; binary builds with `CGO_ENABLED=0`.

- [ ] **Step 5: Commit**

```bash
git add engine/integration_test.go
git commit -m "test(engine): closed-loop integration vs mock LiteLLM + crash-resume"
```

---

## Self-review (run before execution)

**Spec coverage (against `2026-06-08-sloppy-joe-v0-design.md`):**
- §3 architecture (off hot path) → engine never calls inference; only admin. ✓
- §4 modules → every package present except `ingest` (deferred to Plan 2, OTLP) and the daemon `cmd/sloppyd` (Plan 2). **Gap (intentional, documented):** Plan 1 ships the library + `cmd/sloppy` skeleton; `ingest`/`sloppyd`/cost-ledger/replay-CLI are Plan 2.
- §5 data flow → observe(inject/signal)→decide(reconcile)→act(actuator)→record(audit)→revert(actuator.Revert exists; auto-revert scheduler is Plan 2). **Gap (intentional):** TTL auto-revert scheduling is Plan 2; Revert is implemented and unit-tested now.
- §6 rule language (YAML+CEL) → Tasks 8–10. ✓
- §7 durability (at-least-once + idempotent + receipts) → Task 14 + Task 16, with the at-least-once boundary documented. ✓
- §8 cost ledger → **deferred to Plan 2**; Plan 1 carries spend in the signal. Documented in plan goal.
- §9 state (SQLite, hash-chain audit) → Tasks 6–7. ✓
- §10 security (broker, provider keys never here, capability default-deny) → Tasks 5, 11. Backpressure (`intent_budget`) → **Plan 2** (Plan 1 governs via `dry_run` + idempotency). Documented.
- §11 actuators (route_override, open_issue, page) → Tasks 12–13. ✓
- §14 testing (unit, hash-chain, idempotency, integration vs LiteLLM mock) → throughout. Replay-golden CLI → Plan 2. ✓ (unit/integration), partial (CLI replay).

**Placeholder scan:** No "TBD/implement later" inside task steps; every code step has real code. The three `NOTE` blocks are explicit cross-task/scope callouts, not placeholders.

**Type consistency:** `core.RemediationIntent` gains `DryRun` in Task 14 (called out). `Actuator` interface (`Capabilities/Apply/Revert`) is consistent across Tasks 11–13. `state.Store` methods (`IsIntentApplied/MarkIntentApplied/AppendAudit/VerifyAudit/Close`) are consistent across Tasks 7, 14, 16. `rules.NewReconciler/Reconcile` consistent across Tasks 10, 14, 16.

## Plan series (follow-on)

- **Plan 2 — Ingest + Ledger + Daemon:** OTLP push receiver; LiteLLM spend poller; derived Cost Ledger + price book; `sloppyd` continuous reconcile; `for:`-window + TTL auto-revert scheduler; wire CLI (`up`/`rules apply`/`audit tail`/`inject`) to the engine; `intent_budget` backpressure + jitter.
- **Plan 3 — Replay + signal breadth:** `sloppy test --replay` golden engine; latency/error-rate + fallback-fired signals; `state.*` CEL fields; `doctor` capability probing.
- **Plan 4 — Hardening + conformance:** fail-open/closed knob surfaces; Actuator conformance suite (published); persisted signing key; OTel self-telemetry export.

---

## Refinements applied during implementation (2026-06-08)

Implemented autonomously after an adversarial spec/plan review. Deltas from the tasks above:

1. **Signing wired into the loop.** The `engine` signs each intent's canonical bytes (and the receipt) via `intent.Signer`; the applied intent carries a verifiable ed25519 signature, recorded (truncated) in the audit detail. (Plan 1 originally built the signer but left it unused.) Persisted keys + Sigstore remain Plan 4.
2. **One audit hash implementation.** Consolidated to a single `state.ChainHash(ts, kind, detail, prev)` used by both append and verify (no MemAudit/SQLite duplication); a timestamp is part of the chain.
3. **Functional CLI.** `sloppy inject [--rules dir] [--db path] <signal.json>` runs the loop and prints per-intent outcomes; `sloppy audit tail [--db path]` prints the verified chain. Default actuator is a logging actuator (demoable with no live gateway); set `SLOPPY_LITELLM_URL` (+ `SLOPPY_TOKEN_LITELLM`) to wire the real LiteLLM `route_override`. Closes the spec's "10-minute-via-CLI" gap inside Plan 1.
4. **Honest durability test.** The integration test is labelled *restart* (clean shutdown) and documents the at-least-once double-fire boundary; `route_override` is naturally idempotent, incident-scoped dedup for non-idempotent actions (`open_issue`) is Plan 2.
5. **Apache-2.0 LICENSE** added (promised by the spec, was missing).

**Engine API note:** `engine.New(rec, reg, store, signer)` and `Handle(ctx, sig) ([]Result, error)` (rich per-intent `Result`s) supersede the `New(rec,reg,store)` / `(int, error)` signatures sketched in Tasks 14–16.

**Verification:** `go vet ./...` clean; `go test ./...` green (9 packages); `CGO_ENABLED=0 go build` produces a static `bin/sloppy`; end-to-end demo (inject → idempotent replay → verified audit chain) passes through the real binary.

**Deferred (unchanged):** OTLP ingest + cost ledger + daemon + `for:`-window + TTL auto-revert → Plan 2; replay-CLI + signal breadth + `state.*` CEL fields → Plan 3; fail-open/closed surface + conformance suite + persisted signing key + OTel self-telemetry → Plan 4.
