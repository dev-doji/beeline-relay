// Command api runs the Beeline Relay HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/agent"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/auth"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/catalog"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/config"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/httpapi"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/near"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payment"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/playground"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/quote"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Local convenience only: real environment variables always take precedence.
	_ = godotenv.Load()

	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.MigrateOnStart {
		if err := storage.Migrate(pool); err != nil {
			return err
		}
		log.Info("migrations applied")
	}

	pgCfg, err := playground.FromEnv(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("playground config: %w", err)
	}
	var pg *playground.Service
	var inv *invoices.Service
	if pgCfg != nil {
		pg = &playground.Service{
			Cfg:   *pgCfg,
			Chain: near.NewClient(envOr("NEAR_RPC_URL", "https://rpc.testnet.fastnear.com"), 60*time.Second),
			Now:   time.Now,
		}
		if key := os.Getenv("NEAR_AI_API_KEY"); key != "" {
			ai := nearai.NewClient(os.Getenv("NEAR_AI_API_BASE_URL"), key, 120*time.Second)
			pg.Reviewer = ai
			pg.Agent = &agent.Agent{
				LLM:   ai,
				Model: envOr("NEAR_AI_AGENT_MODEL", "z-ai/glm-5.3-flash"),
				Contacts: []agent.Contact{
					{Name: pgCfg.Approved.Label + " (approved supplier)", Account: pgCfg.Approved.Account},
					{Name: pgCfg.Vendor.Label + " (vendor)", Account: pgCfg.Vendor.Account},
				},
			}
		}
		inv = &invoices.Service{DB: pool, Issuer: pgCfg.InvoiceIssuer, Asset: "nep141:" + pgCfg.Token, TTL: 7 * 24 * time.Hour, Now: time.Now}
		log.Info("playground enabled", "vault", pgCfg.Vault, "live_ai", pg.Reviewer != nil, "invoice_issuer", inv.IssuerPublicKeyHex())
	}

	cat := catalog.Default()
	provider := intents.NewClient(cfg.IntentsBaseURL, cfg.IntentsAPIKey, 15*time.Second)
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:            log,
		DB:                pool,
		AllowedOrigins:    cfg.CORSAllowedOrigins,
		Auth:              &auth.Store{DB: pool},
		Catalog:           cat,
		Quotes:            &quote.Service{DB: pool, Provider: provider, Catalog: cat, Now: time.Now},
		Payments:          &payment.Service{DB: pool, Now: time.Now},
		Environment:       "mainnet",
		Playground:        pg,
		PlaygroundLimiter: NewLimiterFromEnv(),
		Invoices:          inv,
		RouteVerification: httpapi.RouteVerification{
			Status: "not_verified",
			Notes:  "No end-to-end payment has been run on this deployment yet. See docs/NEAR_INTENTS_ROUTE.md.",
		},
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      180 * time.Second, // playground runs wait for a NEAR AI review and a testnet transaction
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// NewLimiterFromEnv caps public playground runs: PLAYGROUND_RUNS_PER_MINUTE per client
// (default 4) and PLAYGROUND_RUNS_PER_DAY in total (default 300).
func NewLimiterFromEnv() *httpapi.RunLimiter {
	perMin, perDay := 4, 300
	if v, err := strconv.Atoi(os.Getenv("PLAYGROUND_RUNS_PER_MINUTE")); err == nil && v > 0 {
		perMin = v
	}
	if v, err := strconv.Atoi(os.Getenv("PLAYGROUND_RUNS_PER_DAY")); err == nil && v > 0 {
		perDay = v
	}
	return httpapi.NewRunLimiter(perMin, time.Minute, perDay)
}
