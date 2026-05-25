package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/silhuzz/cexyrouter/internal/api"
)

const handlerTimeout = 8 * time.Second

type handler struct {
	deps api.Deps
}

type freshnessItem struct {
	ExchangeSlug        string     `json:"exchange"`
	ExchangeName        string     `json:"exchange_name"`
	LastSuccessfulPoll  *time.Time `json:"last_successful_poll"`
	LastAttempt         *time.Time `json:"last_attempt"`
	ConsecutiveFailures int32      `json:"consecutive_failures"`
}

type metricsResponse struct {
	Runtime runtimeMetrics `json:"runtime"`
}

type runtimeMetrics struct {
	GoRoutines int    `json:"goroutines"`
	GoVersion  string `json:"go_version"`
}

func Mount(r chi.Router, deps api.Deps) {
	h := handler{deps: deps}
	r.Get("/v1/adapters/freshness", h.listAdapterFreshness)
	r.Get("/v1/internal/metrics", h.metrics)
}

func (h handler) listAdapterFreshness(w http.ResponseWriter, r *http.Request) {
	if h.deps.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "database pool is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), handlerTimeout)
	defer cancel()

	rows, err := h.deps.DB.Query(ctx, `
		SELECT
			e.slug,
			e.name,
			af.last_successful_poll,
			af.last_attempt,
			af.consecutive_failures
		FROM exchanges e
		JOIN adapter_freshness af ON af.exchange_id = e.id
		WHERE af.last_successful_poll IS NOT NULL
		   OR af.last_attempt IS NOT NULL
		   OR af.last_error IS NOT NULL
		ORDER BY e.slug
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database_query_failed", "failed to list adapter freshness")
		return
	}
	defer rows.Close()

	items := make([]freshnessItem, 0)
	for rows.Next() {
		var item freshnessItem
		var lastSuccessfulPoll pgtype.Timestamptz
		var lastAttempt pgtype.Timestamptz
		var consecutiveFailures pgtype.Int4
		if err := rows.Scan(
			&item.ExchangeSlug,
			&item.ExchangeName,
			&lastSuccessfulPoll,
			&lastAttempt,
			&consecutiveFailures,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "database_scan_failed", "failed to read adapter freshness")
			return
		}
		item.LastSuccessfulPoll = timestamptzPtr(lastSuccessfulPoll)
		item.LastAttempt = timestamptzPtr(lastAttempt)
		if consecutiveFailures.Valid {
			item.ConsecutiveFailures = consecutiveFailures.Int32
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "database_rows_failed", "failed to list adapter freshness")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h handler) metrics(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(h.deps.Config.InternalMetricsToken)
	if token == "" {
		writeError(w, http.StatusNotFound, "not_found", "metrics endpoint is disabled")
		return
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "valid metrics token required")
		return
	}

	writeJSON(w, http.StatusOK, metricsResponse{
		Runtime: runtimeMetrics{
			GoRoutines: runtime.NumGoroutine(),
			GoVersion:  runtime.Version(),
		},
	})
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
