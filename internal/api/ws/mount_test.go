package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/silhuzz/cexyrouter/internal/api"
	"github.com/silhuzz/cexyrouter/internal/config"
)

func TestMountRequiresWebSocketUpgrade(t *testing.T) {
	router := chi.NewRouter()
	Mount(router, api.Deps{
		Config: config.APIConfig{CORSAllowedOrigins: []string{"https://demo.example"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	req.Header.Set("Origin", "https://demo.example")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusUpgradeRequired)
	}
}

func TestMountRejectsOrigin(t *testing.T) {
	router := chi.NewRouter()
	Mount(router, api.Deps{
		Config: config.APIConfig{CORSAllowedOrigins: []string{"https://demo.example"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	req.Header.Set("Origin", "https://evil.example")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestMountWithoutDBReportsRuntimeUnavailable(t *testing.T) {
	router := chi.NewRouter()
	Mount(router, api.Deps{
		Config: config.APIConfig{CORSAllowedOrigins: []string{"*"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
}
