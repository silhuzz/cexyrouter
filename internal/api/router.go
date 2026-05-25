package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/silhuzz/cexyrouter/internal/config"
	"github.com/silhuzz/cexyrouter/internal/db"
)

type Deps struct {
	Context context.Context
	DB      *pgxpool.Pool
	Config  config.APIConfig
}

type MountFunc func(r chi.Router, deps Deps)

func NewRouter(deps Deps, mounts ...MountFunc) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.Config.CORSAllowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	if deps.Config.RateLimitPerMin > 0 {
		r.Use(httprate.LimitByIP(deps.Config.RateLimitPerMin, time.Minute))
	}

	MountHealth(r, deps)
	MountFoundation(r, deps)
	for _, mount := range mounts {
		mount(r, deps)
	}
	return r
}

func MountHealth(r chi.Router, deps Deps) {
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := withHandlerTimeout(r)
		defer cancel()

		if err := db.Health(ctx, deps.DB); err != nil {
			slog.Warn("healthz database check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func MountFoundation(r chi.Router, _ Deps) {
	r.Get("/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"name":   "cex-router",
			"status": "foundation",
		})
	})
}

func withHandlerTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 8*time.Second)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
