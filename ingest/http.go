// Package ingest exposes an HTTP receiver that feeds signals + usage into the loop.
// This is the v0 ingest surface; an OTLP-collector → /v1/usage shim is a thin adapter.
package ingest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sloppyjoe/sloppy/core"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
)

// Server adapts HTTP requests into engine + ledger calls.
type Server struct {
	engine  *engine.Engine
	ledger  *ledger.CostLedger // optional; nil disables /v1/usage
	metrics *metrics.Registry  // optional; powers /status
}

// NewServer builds the ingest server.
func NewServer(e *engine.Engine, l *ledger.CostLedger) *Server {
	return &Server{engine: e, ledger: l}
}

// SetMetrics attaches a metrics registry to expose at /status.
func (s *Server) SetMetrics(m *metrics.Registry) *Server {
	s.metrics = m
	return s
}

// Handler returns the configured HTTP mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/signals", s.handleSignal)
	mux.HandleFunc("/v1/usage", s.handleUsage)
	mux.HandleFunc("/v1/otlp/metrics", s.handleOTLP)
	mux.HandleFunc("/status", s.handleStatus)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	snap := map[string]int64{}
	if s.metrics != nil {
		snap = s.metrics.Snapshot()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

type signalResp struct {
	Applied int             `json:"applied"`
	Results []resultSummary `json:"results"`
}

type resultSummary struct {
	Outcome string `json:"outcome"`
	Kind    string `json:"kind"`
	Target  string `json:"target"`
}

func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var sig core.Signal
	if err := json.NewDecoder(r.Body).Decode(&sig); err != nil {
		http.Error(w, "bad signal json: "+err.Error(), http.StatusBadRequest)
		return
	}
	results, err := s.engine.Handle(r.Context(), sig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := signalResp{}
	for _, res := range results {
		if res.Outcome == engine.OutApplied {
			resp.Applied++
		}
		resp.Results = append(resp.Results, resultSummary{
			Outcome: string(res.Outcome), Kind: string(res.Intent.Kind), Target: res.Intent.Target,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type usageReq struct {
	Tenant       string `json:"tenant"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	At           string `json:"at"` // optional RFC3339
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.ledger == nil {
		http.Error(w, "ledger disabled", http.StatusServiceUnavailable)
		return
	}
	var u usageReq
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "bad usage json: "+err.Error(), http.StatusBadRequest)
		return
	}
	at := time.Now().UTC()
	if u.At != "" {
		if parsed, err := time.Parse(time.RFC3339, u.At); err == nil {
			at = parsed
		}
	}
	if err := s.ledger.Record(r.Context(), u.Tenant, u.Model, u.InputTokens, u.OutputTokens, at); err != nil {
		http.Error(w, "record error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
