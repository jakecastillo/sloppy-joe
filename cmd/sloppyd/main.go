// Command sloppyd is the Sloppy Joe daemon: HTTP ingest + continuous TTL auto-revert.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/ee"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/ingest"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/secrets"
)

// buildEngine wires the loop from on-disk config. Returns a cleanup closer.
func buildEngine(rulesPath, dbPath, pricebookPath, keyPath, storeKind, redisAddr string, failClosed bool, out io.Writer, logger *slog.Logger) (*engine.Engine, *ledger.CostLedger, *metrics.Registry, func(), error) {
	rs, err := config.LoadRules(rulesPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	st, err := config.OpenStore(storeKind, dbPath, redisAddr)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	signer, err := intent.LoadOrCreateSigner(keyPath)
	if err != nil {
		st.Close()
		return nil, nil, nil, nil, err
	}
	var pb ledger.PriceBook
	if pricebookPath != "" {
		b, err := os.ReadFile(pricebookPath)
		if err != nil {
			st.Close()
			return nil, nil, nil, nil, err
		}
		if pb, err = ledger.LoadPriceBook(b); err != nil {
			st.Close()
			return nil, nil, nil, nil, err
		}
	}
	l := ledger.New(pb, st)
	m := metrics.New()
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Log{W: out})
	if url := os.Getenv("SLOPPY_LITELLM_URL"); url != "" {
		br := secrets.NewEnvBroker([]string{"litellm"})
		reg.Register(actuator.NewLiteLLM(url, func() (string, error) { return br.Get("litellm") }))
	}
	fm := engine.FailOpen
	if failClosed {
		fm = engine.FailClosed
	}
	e := engine.New(rec, reg, st, signer, engine.WithLedger(l), engine.WithMetrics(m), engine.WithFailMode(fm), engine.WithLogger(logger))
	return e, l, m, func() { st.Close() }, nil
}

// serve runs the ingest HTTP server + the TTL-revert ticker until ctx is cancelled.
func serve(ctx context.Context, ln net.Listener, e *engine.Engine, l *ledger.CostLedger, m *metrics.Registry, authz *ee.Authorizer, revertEvery time.Duration, logger *slog.Logger) error {
	h := ingest.NewServer(e, l).SetMetrics(m).Handler()
	if authz != nil {
		h = authz.Middleware(h)
	}
	srv := &http.Server{Handler: h}

	go func() {
		ticker := time.NewTicker(revertEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().UTC()
				if n, err := e.ProcessDueReverts(ctx, now); err == nil && n > 0 {
					logger.Info("reverted expired intents", "count", n)
				}
				_ = e.PruneUsage(ctx, now.Add(-48*time.Hour))
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
	}()

	logger.Info("sloppyd listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	addr := flag.String("addr", ":8723", "listen address")
	rulesPath := flag.String("rules", "rules", "rules dir or file")
	dbPath := flag.String("db", "sloppy.db", "sqlite db path")
	pricebookPath := flag.String("pricebook", "", "price book yaml (optional)")
	keyPath := flag.String("key", "sloppy.key", "ed25519 signing key file (created if absent)")
	failClosed := flag.Bool("fail-closed", false, "refuse to act when state is unavailable")
	authOn := flag.Bool("auth", false, "require API-key RBAC on the HTTP API (keys via SLOPPY_API_KEYS)")
	store := flag.String("store", "sqlite", "state backend: sqlite|redis")
	redisAddr := flag.String("redis-addr", "", "redis address host:port (when --store=redis)")
	logFormat := flag.String("log-format", "text", "log format: text|json")
	revertEvery := flag.Duration("revert-interval", 30*time.Second, "TTL revert scan interval")
	flag.Parse()

	var lh slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if *logFormat == "json" {
		lh = slog.NewJSONHandler(os.Stdout, nil)
	}
	logger := slog.New(lh)

	var authz *ee.Authorizer
	if *authOn {
		authz = ee.LoadFromEnv()
	}

	e, l, m, cleanup, err := buildEngine(*rulesPath, *dbPath, *pricebookPath, *keyPath, *store, *redisAddr, *failClosed, os.Stdout, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer cleanup()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, ln, e, l, m, authz, *revertEvery, logger); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}
