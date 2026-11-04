// Package httpapi wires HTTP routing, middleware and handlers for the Beeline Relay API.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Pinger reports whether a dependency is reachable. The database pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Logger         *slog.Logger
	DB             Pinger
	AllowedOrigins []string
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
		writeError(w, r, http.StatusNotFound, CodeNotFound, "Route not found.")
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", healthHandler(d.DB))
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
					writeError(w, r, http.StatusInternalServerError, CodeInternal, "Internal error.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
