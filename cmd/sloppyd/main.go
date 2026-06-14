// Command sloppyd is the Sloppy Joe daemon: HTTP ingest + continuous TTL auto-revert.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sloppyjoe/sloppy/bootstrap"
	"github.com/sloppyjoe/sloppy/config"
	"github.com/sloppyjoe/sloppy/ee"
	"github.com/sloppyjoe/sloppy/engine"
	"github.com/sloppyjoe/sloppy/ingest"
	"github.com/sloppyjoe/sloppy/ledger"
	"github.com/sloppyjoe/sloppy/metrics"
)

// serve runs the ingest HTTP server + the TTL-revert ticker until ctx is cancelled.
func serve(ctx context.Context, ln net.Listener, e *engine.Engine, l *ledger.CostLedger, m *metrics.Registry, authz *ee.Authorizer, revertEvery time.Duration, logger *slog.Logger) error {
	h := ingest.NewServer(e, ingest.WithLedger(l), ingest.WithMetrics(m)).Handler()
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
				runRevertScan(ctx, e, m, logger, time.Now().UTC())
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

// checkBindAuth enforces the fail-safe bind policy: an unauthenticated control
// plane must not be exposed on a network-reachable address. It returns an error
// when addr is non-loopback (a specific LAN/WAN ip, or the wildcard/empty host
// that listens on every interface) while auth is disabled. Loopback binds and any
// bind with --auth on are allowed.
func checkBindAuth(addr string, authEnabled bool) error {
	if authEnabled {
		return nil
	}
	if addrIsLoopback(addr) {
		return nil
	}
	return fmt.Errorf("refusing to bind %q without --auth: that address is reachable from the network and the API would be UNAUTHENTICATED. Enable --auth (with SLOPPY_API_KEYS), or bind a loopback address (127.0.0.1 / [::1])", addr)
}

// addrIsLoopback reports whether a listen address binds only the loopback
// interface. An empty host (e.g. ":8723") or a wildcard (0.0.0.0 / ::) binds every
// interface and is therefore NOT loopback. A hostname that resolves entirely to
// loopback ips (e.g. "localhost") counts as loopback.
func addrIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port (or malformed): treat the whole string as a host.
		host = addr
	}
	if host == "" {
		return false // ":8723" => all interfaces
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	// Hostname: loopback only if every resolved address is loopback.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}

// logAuthState emits a LOUD, unambiguous startup line for the API auth posture so
// an operator can never silently run an open control plane — or, worse, an
// auth-on-but-no-keys instance where every protected route is locked out. The two
// hazardous states (auth disabled; auth enabled with zero keys) log at WARN; the
// healthy state logs at INFO.
func logAuthState(logger *slog.Logger, authEnabled bool, keyCount int) {
	switch {
	case !authEnabled:
		logger.Warn("API auth disabled: the HTTP API is UNAUTHENTICATED — anyone who can reach it can drive the control loop. Enable --auth (with SLOPPY_API_KEYS) before exposing it",
			"auth", false)
	case keyCount == 0:
		logger.Warn("API auth enabled but NO api keys are configured: every protected route is unreachable. Set SLOPPY_API_KEYS",
			"auth", true, "keys", 0)
	default:
		logger.Info("API auth enabled", "auth", true, "keys", keyCount)
	}
}

// runRevertScan performs one tick of the TTL auto-revert safety net: it processes
// due reverts and prunes stale usage. Both are best-effort against the store, but
// neither error may be swallowed silently — a store outage that disables the
// safety net must be observable, so each failure logs a Warn and bumps
// revert_scan_failed. Extracted from the ticker loop so the failure paths are
// unit-testable.
func runRevertScan(ctx context.Context, e *engine.Engine, m *metrics.Registry, logger *slog.Logger, now time.Time) {
	if n, err := e.ProcessDueReverts(ctx, now); err != nil {
		m.Inc("revert_scan_failed")
		logger.Warn("process due reverts failed", "err", err)
	} else if n > 0 {
		logger.Info("reverted expired intents", "count", n)
	}
	if err := e.PruneUsage(ctx, now.Add(-48*time.Hour)); err != nil {
		m.Inc("revert_scan_failed")
		logger.Warn("prune usage failed", "err", err)
	}
}

func main() {
	cfgPath := flag.String("config", "sloppy.yaml", "path to sloppy.yaml")
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
	revertEvery := flag.String("revert-interval", "30s", "TTL revert scan interval")
	flag.Parse()

	// Only flags the user explicitly set become overrides (precedence: flag > env > file).
	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	ov := config.FlagOverrides{}
	if set["addr"] {
		ov.Addr = addr
	}
	if set["rules"] {
		ov.Rules = rulesPath
	}
	if set["db"] {
		ov.DBPath = dbPath
	}
	if set["pricebook"] {
		ov.Pricebook = pricebookPath
	}
	if set["key"] {
		ov.SigningKey = keyPath
	}
	if set["store"] {
		ov.Store = store
	}
	if set["redis-addr"] {
		ov.RedisAddr = redisAddr
	}
	if set["log-format"] {
		ov.LogFormat = logFormat
	}
	if set["revert-interval"] {
		ov.RevertInterval = revertEvery
	}
	if set["fail-closed"] {
		ov.FailClosed = failClosed
	}
	if set["auth"] {
		ov.Auth = authOn
	}

	f, existed, err := config.LoadFile(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	eff := config.Resolve(f, existed, ov, os.Getenv)

	var lh slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if eff.Engine.LogFormat == "json" {
		lh = slog.NewJSONHandler(os.Stdout, nil)
	}
	logger := slog.New(lh)

	var authz *ee.Authorizer
	if eff.Auth.Enabled {
		authz = ee.LoadFromEnv()
	}

	// Make the auth posture LOUD and refuse to expose an unauthenticated API on a
	// network-reachable address. Both run before we open any port.
	logAuthState(logger, eff.Auth.Enabled, authz.KeyCount())
	if err := checkBindAuth(eff.Server.Addr, eff.Auth.Enabled); err != nil {
		fmt.Fprintln(os.Stderr, "bind guard:", err)
		os.Exit(1)
	}

	e, l, m, cleanup, err := bootstrap.BuildEngine(eff, os.Stdout, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer cleanup()

	revertEveryDur, err := eff.RevertInterval()
	if err != nil {
		revertEveryDur = 30 * time.Second
	}

	ln, err := net.Listen("tcp", eff.Server.Addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, ln, e, l, m, authz, revertEveryDur, logger); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}
