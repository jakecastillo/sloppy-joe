// Command sloppyd is the Sloppy Joe daemon: HTTP ingest + continuous TTL auto-revert.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sloppyjoe/sloppy/actuator"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/ingest"
	"github.com/sloppyjoe/sloppy/intent"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/rules"
	"github.com/sloppyjoe/sloppy/secrets"
	"github.com/sloppyjoe/sloppy/state"
)

// buildEngine wires the loop from on-disk config. Returns a cleanup closer.
func buildEngine(rulesPath, dbPath, pricebookPath string, out io.Writer) (*engine.Engine, *ledger.CostLedger, func(), error) {
	rs, err := config.LoadRules(rulesPath)
	if err != nil {
		return nil, nil, nil, err
	}
	rec, err := rules.NewReconciler(rs)
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := state.OpenSQLite(dbPath)
	if err != nil {
		return nil, nil, nil, err
	}
	signer, err := intent.NewEd25519Signer()
	if err != nil {
		st.Close()
		return nil, nil, nil, err
	}
	var pb ledger.PriceBook
	if pricebookPath != "" {
		b, err := os.ReadFile(pricebookPath)
		if err != nil {
			st.Close()
			return nil, nil, nil, err
		}
		if pb, err = ledger.LoadPriceBook(b); err != nil {
			st.Close()
			return nil, nil, nil, err
		}
	}
	l := ledger.New(pb)
	reg := actuator.NewRegistry()
	reg.Register(&actuator.Log{W: out})
	if url := os.Getenv("SLOPPY_LITELLM_URL"); url != "" {
		br := secrets.NewEnvBroker([]string{"litellm"})
		reg.Register(actuator.NewLiteLLM(url, func() (string, error) { return br.Get("litellm") }))
	}
	e := engine.New(rec, reg, st, signer, engine.WithLedger(l))
	return e, l, func() { st.Close() }, nil
}

// serve runs the ingest HTTP server + the TTL-revert ticker until ctx is cancelled.
func serve(ctx context.Context, ln net.Listener, e *engine.Engine, l *ledger.CostLedger, revertEvery time.Duration, out io.Writer) error {
	srv := &http.Server{Handler: ingest.NewServer(e, l).Handler()}

	go func() {
		ticker := time.NewTicker(revertEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := e.ProcessDueReverts(ctx, time.Now().UTC()); err == nil && n > 0 {
					fmt.Fprintf(out, "reverted %d expired intent(s)\n", n)
				}
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
	}()

	fmt.Fprintf(out, "🥪 sloppyd listening on %s\n", ln.Addr())
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
	revertEvery := flag.Duration("revert-interval", 30*time.Second, "TTL revert scan interval")
	flag.Parse()

	e, l, cleanup, err := buildEngine(*rulesPath, *dbPath, *pricebookPath, os.Stdout)
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
	if err := serve(ctx, ln, e, l, *revertEvery, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}
