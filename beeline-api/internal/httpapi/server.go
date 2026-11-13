// Package httpapi wires HTTP routing, middleware and handlers for the Beeline Relay API.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/accounts"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/auth"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/catalog"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/guard"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payin"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payment"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/playground"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/quote"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

// Pinger reports whether a dependency is reachable. The database pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Authenticator interface {
	Authenticate(ctx context.Context, key string) (auth.Principal, error)
}

// RouteVerification is reported verbatim by /capabilities. It must only say "verified"
// after a real end-to-end payment has been run and recorded.
type RouteVerification struct {
	Status         string     `json:"status"`
	LastVerifiedAt *time.Time `json:"last_verified_at"`
	Notes          string     `json:"notes"`
}

type Deps struct {
	Logger            *slog.Logger
	DB                Pinger
	AllowedOrigins    []string
	Auth              Authenticator
	Catalog           *catalog.Catalog
	Quotes            *quote.Service
	Payments          *payment.Service
	Environment       string
	RouteVerification RouteVerification
	// Playground is the public testnet demo; nil disables its endpoints.
	Playground        *playground.Service
	PlaygroundLimiter *RunLimiter
	Invoices          *invoices.Service
	// Switch (offramp to a bank) and Intents (route pricing) back the cash-out preview; nil disables it.
	Switch  *switchramp.Client
	Intents intents.Provider
	// CashoutLive allows real (live-key) payout sessions up to CashoutLiveMaxUSDC. Off by default.
	CashoutLive        bool
	CashoutLiveMaxUSDC string
	// Accounts and Privy back the signed-in app (/me); nil disables it.
	Accounts *accounts.Store
	Privy    *accounts.Verifier
	// Chat is the NEAR AI model behind Bee, the signed-in assistant; nil falls back to the command parser.
	Chat      accounts.Completer
	ChatModel string
	// Claims verifies wallet payments against the chain; nil disables POST /playground/invoices/{id}/claims.
	Claims ClaimVerifier
	// ClaimsMainnet verifies wallet payments for real-money (mainnet) invoices; nil refuses them.
	ClaimsMainnet ClaimVerifier
	// PayIn lets customers pay a real-money invoice from any chain through NEAR Intents; nil disables it.
	PayIn *payin.Service
	// Storage checks that a mainnet account can hold USDC (is registered with the token contract); nil skips the check.
	Storage StorageChecker
	// Guard is the AI fraud analyst that reviews payment events in the background; nil switches it off.
	Guard *guard.Service
}

// StorageChecker is a read-only NEP-145 registration check on NEAR mainnet.
type StorageChecker interface {
	StorageRegistered(ctx context.Context, token, account string) (bool, error)
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(d.Logger))
	r.Use(recoverer(d.Logger))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: d.AllowedOrigins,
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions},
		AllowedHeaders: []string{"Authorization", "Content-Type", "Idempotency-Key"},
		ExposedHeaders: []string{"X-Request-Id"},
		MaxAge:         300,
	}))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusNotFound, apperr.CodeNotFound, "Route not found.")
	})

	h := &handlers{d: d}
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", healthHandler(d.DB))
		r.Get("/health/ai", h.healthAI)
		r.Get("/capabilities", h.capabilities)
		r.Get("/networks", h.networks)
		r.Get("/assets", h.assets)
		r.Get("/playground", h.playgroundState)
		r.Post("/playground/runs", h.playgroundRun)
		r.Post("/playground/invoices", h.createInvoice)
		r.Get("/playground/invoices/{id}", h.getInvoice)
		r.Post("/playground/invoices/{id}/pay", h.payInvoice)
		r.Get("/playground/invoices/{id}/attempts", h.invoiceAttempts)
		r.Post("/playground/invoices/{id}/claims", h.claimInvoice)
		r.Get("/playground/invoices/{id}/events", h.invoiceEvents)
		r.Get("/playground/invoices/{id}/guard", h.invoiceGuard)
		r.Get("/payin/options", h.payinOptions)
		r.Post("/playground/invoices/{id}/payin", h.payinCreate)
		r.Get("/playground/invoices/{id}/payin", h.payinStatus)
		r.Get("/playground/attempts", h.attackLog)
		r.Post("/cashout/quote", h.cashoutQuote)
		r.Post("/cashout/start", h.cashoutStart)
		r.Post("/cashout/prepare", h.cashoutPrepare)
		r.Get("/me", h.requireUser(h.meGet))
		r.Put("/me", h.requireUser(h.meUpdate))
		r.Get("/me/invoices", h.requireUser(h.meInvoices))
		r.Get("/me/ledger", h.requireUser(h.meLedger))
		r.Get("/me/messages", h.requireUser(h.meMessages))
		r.Post("/me/chat", h.requireUser(h.meChat))
		r.Post("/me/chat/new", h.requireUser(h.meNewChat))
		r.Post("/cashout/deposit", h.cashoutDeposit)
		r.Get("/cashout/{reference}", h.cashoutStatus)

		r.Group(func(r chi.Router) {
			r.Use(requireAuth(d.Auth, d.Logger))
			r.Post("/quotes", h.createQuote)
			r.Get("/quotes/{id}", h.getQuote)
			r.Post("/payments", h.createPayment)
			r.Get("/payments", h.listPayments)
			r.Get("/payments/{id}", h.getPayment)
			r.Get("/payments/{id}/events", h.paymentEvents)
		})
	})
	return r
}

// healthHandler reports service health with safe booleans only — no connection details.
func healthHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Neon computes scale to zero; a cold start can take a few seconds.
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		dbOK := db != nil && db.Ping(ctx) == nil
		status, code := "ok", http.StatusOK
		if !dbOK {
			status, code = "degraded", http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{
			"status": status,
			"checks": map[string]bool{"database": dbOK},
		})
	}
}

// requireAuth accepts "Authorization: Bearer blr_…" and stores the principal in the context.
func requireAuth(a Authenticator, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || key == "" || a == nil {
				writeError(w, r, http.StatusUnauthorized, apperr.CodeUnauthorized, "Missing or malformed Authorization header.")
				return
			}
			p, err := a.Authenticate(r.Context(), strings.TrimSpace(key))
			if errors.Is(err, auth.ErrInvalidKey) {
				writeError(w, r, http.StatusUnauthorized, apperr.CodeUnauthorized, "Invalid or revoked API key.")
				return
			}
			if err != nil {
				writeAppError(w, r, log, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
		})
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			w.Header().Set("X-Request-Id", middleware.GetReqID(r.Context()))
			next.ServeHTTP(ww, r)
			// Log path only, never query strings or headers: they can carry secrets.
			log.Info("http_request",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "request_id", middleware.GetReqID(r.Context()), "panic", rec)
					writeError(w, r, http.StatusInternalServerError, apperr.CodeInternal, "Internal error.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
