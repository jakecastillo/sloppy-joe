package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/state"
)

// seedReportDB builds a store with two audit entries (one of each kind) and a
// small usage history, then returns its path. The report command reads this back
// over the existing public queries only.
func seedReportDB(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "report.db")
	st, err := state.OpenSQLite(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := t.Context()
	if _, err := st.AppendAudit(ctx, "intent.applied", "reroute acme"); err != nil {
		t.Fatalf("audit applied: %v", err)
	}
	if _, err := st.AppendAudit(ctx, "intent.applied", "reroute beta"); err != nil {
		t.Fatalf("audit applied: %v", err)
	}
	if _, err := st.AppendAudit(ctx, "intent.reverted", "restore acme"); err != nil {
		t.Fatalf("audit reverted: %v", err)
	}
	if err := st.RecordUsage(ctx, "acme", "gpt-4o", 1.50, time.Now()); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if err := st.RecordUsage(ctx, "acme", "gpt-4o", 2.25, time.Now()); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestReportTable(t *testing.T) {
	db := seedReportDB(t)
	var buf bytes.Buffer
	if code := run([]string{"report", "--db", db, "--tenant", "acme", "--since", "1h"}, &buf); code != 0 {
		t.Fatalf("report exit %d: %s", code, buf.String())
	}
	got := buf.String()
	for _, want := range []string{
		"audit:",
		"entries   3",
		"verified",
		"intent.applied",
		"intent.reverted",
		"spend:",
		"tenant    acme",
		"3.75", // 1.50 + 2.25
	} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q:\n%s", want, got)
		}
	}
}

func TestReportJSON(t *testing.T) {
	db := seedReportDB(t)
	var buf bytes.Buffer
	if code := run([]string{"report", "--db", db, "--format", "json", "--tenant", "acme", "--since", "1h"}, &buf); code != 0 {
		t.Fatalf("report exit %d: %s", code, buf.String())
	}
	var data reportData
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, buf.String())
	}
	if data.AuditCount != 3 {
		t.Errorf("audit_count = %d, want 3", data.AuditCount)
	}
	if !data.ChainVerified {
		t.Error("chain_verified = false, want true on a fresh chain")
	}
	if data.ByKind["intent.applied"] != 2 {
		t.Errorf("by_kind[intent.applied] = %d, want 2", data.ByKind["intent.applied"])
	}
	if data.ByKind["intent.reverted"] != 1 {
		t.Errorf("by_kind[intent.reverted] = %d, want 1", data.ByKind["intent.reverted"])
	}
	if data.Tenant != "acme" {
		t.Errorf("tenant = %q, want acme", data.Tenant)
	}
	if data.SpendUSD != 3.75 {
		t.Errorf("spend_usd = %v, want 3.75", data.SpendUSD)
	}
}

func TestReportCSV(t *testing.T) {
	db := seedReportDB(t)
	var buf bytes.Buffer
	if code := run([]string{"report", "--db", db, "--format", "csv", "--tenant", "acme", "--since", "1h"}, &buf); code != 0 {
		t.Fatalf("report exit %d: %s", code, buf.String())
	}
	got := buf.String()
	for _, want := range []string{
		"section,key,value",
		"audit,count,3",
		"audit,chain_verified,true",
		"by_kind,intent.applied,2",
		"by_kind,intent.reverted,1",
		"spend,tenant,acme",
		"spend,usd,3.75",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("csv output missing %q:\n%s", want, got)
		}
	}
}

// An unknown --format must be rejected with a usage exit (2), not silently
// fall through to the table default.
func TestReportBadFormat(t *testing.T) {
	db := seedReportDB(t)
	var buf bytes.Buffer
	if code := run([]string{"report", "--db", db, "--format", "xml"}, &buf); code != 2 {
		t.Fatalf("bad format must exit 2, got %d: %s", code, buf.String())
	}
}

// The report is read-only: a tenant with no usage reports zero spend without error.
func TestReportEmptyTenant(t *testing.T) {
	db := seedReportDB(t)
	var buf bytes.Buffer
	if code := run([]string{"report", "--db", db, "--format", "json", "--tenant", "nobody", "--since", "1h"}, &buf); code != 0 {
		t.Fatalf("report exit %d: %s", code, buf.String())
	}
	var data reportData
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("json unmarshal: %v\n%s", err, buf.String())
	}
	if data.SpendUSD != 0 {
		t.Errorf("spend_usd = %v, want 0 for a tenant with no usage", data.SpendUSD)
	}
}
