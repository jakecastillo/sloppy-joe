package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/sloppyjoe/sloppy/state"
)

// reportData is the read-only snapshot rendered by `sloppy report`. It is built
// entirely from EXISTING store queries (Audit/VerifyAudit + SpendSince) — the
// report adds no new state query and never actuates, so it is safe to run against
// a live database. The JSON form is exactly this struct.
type reportData struct {
	AuditCount    int            `json:"audit_count"`
	ChainVerified bool           `json:"chain_verified"`
	ByKind        map[string]int `json:"by_kind"`
	Tenant        string         `json:"tenant"`
	Since         time.Time      `json:"since"`
	Window        string         `json:"window"`
	SpendUSD      float64        `json:"spend_usd"`
}

// cmdReport renders a read-only operator summary: the audit chain (total entries,
// whether it verifies, and a per-kind breakdown) plus spend-since for one tenant
// over a window. It reuses only public store methods and writes nothing.
func cmdReport(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(out)
	dbPath := fs.String("db", "sloppy.db", "sqlite db path")
	format := fs.String("format", "table", "output format: table|json|csv")
	tenant := fs.String("tenant", "default", "tenant whose spend-since is reported")
	window := fs.Duration("since", 24*time.Hour, "spend-since window (e.g. 1h, 24h)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	switch *format {
	case "table", "json", "csv":
	default:
		fmt.Fprintf(out, "unknown format %q (want table|json|csv)\n", *format)
		return 2
	}

	st, err := state.OpenSQLite(*dbPath)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	ctx := context.Background()
	entries, err := st.Audit(ctx)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	since := time.Now().Add(-*window)
	spend, err := st.SpendSince(ctx, *tenant, since)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}

	data := reportData{
		AuditCount:    len(entries),
		ChainVerified: st.VerifyAudit(ctx),
		ByKind:        countByKind(entries),
		Tenant:        *tenant,
		Since:         since.UTC(),
		Window:        window.String(),
		SpendUSD:      spend,
	}

	switch *format {
	case "json":
		return renderReportJSON(out, data)
	case "csv":
		return renderReportCSV(out, data)
	default:
		return renderReportTable(out, data)
	}
}

// countByKind tallies audit entries per kind so the report can show the chain's
// composition (e.g. how many intent.applied vs intent.reverted) without a new query.
func countByKind(entries []state.AuditEntry) map[string]int {
	by := make(map[string]int, len(entries))
	for _, e := range entries {
		by[e.Kind]++
	}
	return by
}

// sortedKinds returns the by-kind keys in a stable order so table/csv output is
// deterministic (maps iterate randomly in Go).
func sortedKinds(by map[string]int) []string {
	kinds := make([]string, 0, len(by))
	for k := range by {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

func renderReportJSON(out io.Writer, data reportData) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	return 0
}

func renderReportCSV(out io.Writer, data reportData) int {
	w := csv.NewWriter(out)
	rows := [][]string{
		{"section", "key", "value"},
		{"audit", "count", strconv.Itoa(data.AuditCount)},
		{"audit", "chain_verified", strconv.FormatBool(data.ChainVerified)},
	}
	for _, k := range sortedKinds(data.ByKind) {
		rows = append(rows, []string{"by_kind", k, strconv.Itoa(data.ByKind[k])})
	}
	rows = append(rows,
		[]string{"spend", "tenant", data.Tenant},
		[]string{"spend", "since", data.Since.Format(time.RFC3339)},
		[]string{"spend", "window", data.Window},
		[]string{"spend", "usd", strconv.FormatFloat(data.SpendUSD, 'f', -1, 64)},
	)
	if err := w.WriteAll(rows); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 1
	}
	return 0
}

func renderReportTable(out io.Writer, data reportData) int {
	status := "verified ✓"
	if !data.ChainVerified {
		status = "TAMPERED ✗"
	}
	fmt.Fprintln(out, "audit:")
	fmt.Fprintf(out, "  entries   %d\n", data.AuditCount)
	fmt.Fprintf(out, "  chain     %s\n", status)
	fmt.Fprintln(out, "  by-kind:")
	if len(data.ByKind) == 0 {
		fmt.Fprintln(out, "    (none)")
	}
	for _, k := range sortedKinds(data.ByKind) {
		fmt.Fprintf(out, "    %-20s %d\n", k, data.ByKind[k])
	}
	fmt.Fprintln(out, "spend:")
	fmt.Fprintf(out, "  tenant    %s\n", data.Tenant)
	fmt.Fprintf(out, "  window    %s (since %s)\n", data.Window, data.Since.Format(time.RFC3339))
	fmt.Fprintf(out, "  usd       %s\n", strconv.FormatFloat(data.SpendUSD, 'f', -1, 64))
	return 0
}
