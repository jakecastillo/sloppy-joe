// Package ledger derives a priced cost ledger from token-usage events, persisted
// behind a state.Store so spend survives restarts. Cost is absent from the OTel
// GenAI semconv, so Sloppy Joe prices it itself.
package ledger

import (
	"context"
	"time"

	"gopkg.in/yaml.v3"
)

// State keys exposed to CEL `state.*` guards. These are the contract between the
// ledger (producer, in StateFor) and rule conditions (consumer, e.g.
// `state.spend_1h_usd > 10`); naming them keeps the producer and any Go-side
// consumer from drifting against a bare string literal.
const (
	// StateKeySpend1h is the estimated tenant spend over the trailing hour (USD).
	StateKeySpend1h = "spend_1h_usd"
	// StateKeySpend24h is the estimated tenant spend over the trailing 24 hours (USD).
	StateKeySpend24h = "spend_24h_usd"
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

// UsageStore is the persistence the ledger needs (satisfied by state.Store).
type UsageStore interface {
	RecordUsage(ctx context.Context, tenant, model string, cost float64, at time.Time) error
	SpendSince(ctx context.Context, tenant string, since time.Time) (float64, error)
}

// CostLedger prices token usage and persists/queries it via a UsageStore.
// Spend is estimated (best-effort pricing from a static price book).
type CostLedger struct {
	prices PriceBook
	store  UsageStore
}

// New builds a ledger over the given price book and persistence.
func New(pb PriceBook, store UsageStore) *CostLedger {
	if pb == nil {
		pb = PriceBook{}
	}
	return &CostLedger{prices: pb, store: store}
}

// Record prices a usage event (in/out token counts) and persists it.
func (l *CostLedger) Record(ctx context.Context, tenant, model string, inTok, outTok int, at time.Time) error {
	p := l.prices[model]
	cost := float64(inTok)/1000*p.InputPer1K + float64(outTok)/1000*p.OutputPer1K
	return l.store.RecordUsage(ctx, tenant, model, cost, at)
}

// Spend returns total estimated $ for tenant within [now-window, now].
func (l *CostLedger) Spend(ctx context.Context, tenant string, window time.Duration, now time.Time) (float64, error) {
	return l.store.SpendSince(ctx, tenant, now.Add(-window))
}

// StateFor returns CEL `state.*` fields for a tenant (estimated spend windows).
func (l *CostLedger) StateFor(ctx context.Context, tenant string, now time.Time) (map[string]any, error) {
	h, err := l.store.SpendSince(ctx, tenant, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	d, err := l.store.SpendSince(ctx, tenant, now.Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	return map[string]any{StateKeySpend1h: h, StateKeySpend24h: d}, nil
}
