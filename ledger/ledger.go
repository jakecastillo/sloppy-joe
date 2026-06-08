// Package ledger derives a priced cost ledger from token-usage events.
// Cost is absent from the OTel GenAI semconv, so Sloppy Joe prices it itself.
package ledger

import (
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Price is per-1k-token pricing for a model.
type Price struct {
	InputPer1K  float64 `yaml:"input_per_1k"`
	OutputPer1K float64 `yaml:"output_per_1k"`
}

// PriceBook maps model name -> Price.
type PriceBook map[string]Price

// LoadPriceBook parses a YAML price book (model -> {input_per_1k, output_per_1k}).
func LoadPriceBook(b []byte) (PriceBook, error) {
	var pb PriceBook
	if err := yaml.Unmarshal(b, &pb); err != nil {
		return nil, err
	}
	return pb, nil
}

type usage struct {
	tenant string
	at     time.Time
	cost   float64
}

// CostLedger accumulates priced token usage and answers windowed spend queries.
// Estimated: pricing is best-effort from a static price book.
type CostLedger struct {
	mu     sync.Mutex
	prices PriceBook
	events []usage
}

// New builds a ledger with the given price book.
func New(pb PriceBook) *CostLedger {
	if pb == nil {
		pb = PriceBook{}
	}
	return &CostLedger{prices: pb}
}

// Record prices a usage event (in/out token counts) and stores it.
func (l *CostLedger) Record(tenant, model string, inTok, outTok int, at time.Time) {
	p := l.prices[model]
	cost := float64(inTok)/1000*p.InputPer1K + float64(outTok)/1000*p.OutputPer1K
	l.mu.Lock()
	l.events = append(l.events, usage{tenant: tenant, at: at, cost: cost})
	l.mu.Unlock()
}

// Spend returns total estimated $ for tenant within [now-window, now].
func (l *CostLedger) Spend(tenant string, window time.Duration, now time.Time) float64 {
	cutoff := now.Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	var sum float64
	for _, e := range l.events {
		if e.tenant == tenant && !e.at.Before(cutoff) && !e.at.After(now) {
			sum += e.cost
		}
	}
	return sum
}

// StateFor returns CEL `state.*` fields for a tenant (estimated spend windows).
func (l *CostLedger) StateFor(tenant string, now time.Time) map[string]any {
	return map[string]any{
		"spend_1h_usd":  l.Spend(tenant, time.Hour, now),
		"spend_24h_usd": l.Spend(tenant, 24*time.Hour, now),
	}
}
