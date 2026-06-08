package ledger

import (
	"testing"
	"time"
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
	l := New(pb)
	now := time.Unix(1749340800, 0).UTC()
	// 1000 in, 1000 out => 5.0 + 15.0 = 20.0
	l.Record("acme", "gpt-4o", 1000, 1000, now)
	if got := l.Spend("acme", time.Hour, now); got != 20.0 {
		t.Fatalf("want spend 20.0, got %v", got)
	}
	// Other tenant unaffected.
	if got := l.Spend("other", time.Hour, now); got != 0 {
		t.Fatalf("other tenant should be 0, got %v", got)
	}
	// Outside the window.
	old := now.Add(-2 * time.Hour)
	l.Record("acme", "gpt-4o", 1000, 1000, old)
	if got := l.Spend("acme", time.Hour, now); got != 20.0 {
		t.Fatalf("out-of-window usage must not count, got %v", got)
	}
}

func TestStateForExposesSpendFields(t *testing.T) {
	l := New(PriceBook{"m": {InputPer1K: 1, OutputPer1K: 1}})
	now := time.Unix(1749340800, 0).UTC()
	l.Record("acme", "m", 1000, 0, now)
	st := l.StateFor("acme", now)
	if st["spend_1h_usd"].(float64) != 1.0 {
		t.Fatalf("spend_1h_usd wrong: %v", st["spend_1h_usd"])
	}
	if _, ok := st["spend_24h_usd"]; !ok {
		t.Fatal("spend_24h_usd missing")
	}
}
