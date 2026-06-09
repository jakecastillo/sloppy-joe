package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/sloppyjoe/sloppy/state"
)

func TestLoadPriceBookAndRecordSpend(t *testing.T) {
	pb, err := LoadPriceBook([]byte(`
gpt-4o:
  input_per_1k: 5.0
  output_per_1k: 15.0
`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	st, err := state.OpenSQLite(t.TempDir() + "/l.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l := New(pb, st)
	ctx := context.Background()
	now := time.Unix(1749340800, 0).UTC()
	at := now.Add(time.Second)

	// 1000 in, 1000 out => 5.0 + 15.0 = 20.0
	if err := l.Record(ctx, "acme", "gpt-4o", 1000, 1000, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.Spend(ctx, "acme", time.Hour, at); got != 20.0 {
		t.Fatalf("want spend 20.0, got %v", got)
	}
	if got, _ := l.Spend(ctx, "other", time.Hour, at); got != 0 {
		t.Fatalf("other tenant should be 0, got %v", got)
	}
	// Out-of-window usage must not count.
	if err := l.Record(ctx, "acme", "gpt-4o", 1000, 1000, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.Spend(ctx, "acme", time.Hour, at); got != 20.0 {
		t.Fatalf("out-of-window usage must not count, got %v", got)
	}
}

func TestStateForExposesSpendFields(t *testing.T) {
	st, err := state.OpenSQLite(t.TempDir() + "/l2.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l := New(PriceBook{"m": {InputPer1K: 1, OutputPer1K: 1}}, st)
	ctx := context.Background()
	now := time.Unix(1749340800, 0).UTC()
	if err := l.Record(ctx, "acme", "m", 1000, 0, now); err != nil {
		t.Fatal(err)
	}
	s, err := l.StateFor(ctx, "acme", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if s["spend_1h_usd"].(float64) != 1.0 {
		t.Fatalf("spend_1h_usd wrong: %v", s["spend_1h_usd"])
	}
	if _, ok := s["spend_24h_usd"]; !ok {
		t.Fatal("spend_24h_usd missing")
	}
}
