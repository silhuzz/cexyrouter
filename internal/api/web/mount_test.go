package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/silhuzz/cexyrouter/internal/api"
)

func TestMountServesEmbeddedStaticAssets(t *testing.T) {
	r := chi.NewRouter()
	Mount(r, api.Deps{})

	tests := []struct {
		path        string
		status      int
		contentType string
		body        string
	}{
		{path: "/", status: http.StatusOK, contentType: "text/html", body: "CexyRouter"},
		{path: "/app.css", status: http.StatusOK, contentType: "text/css", body: "--bg"},
		{path: "/app.js", status: http.StatusOK, contentType: "javascript", body: "/v1/route-options"},
		{path: "/cexyrouter-logo.svg", status: http.StatusOK, contentType: "image/svg+xml", body: "CexyRouter"},
		{path: "/favicon.svg", status: http.StatusOK, contentType: "image/svg+xml", body: "C&gt;"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != tt.status {
			t.Fatalf("%s status = %d, want %d", tt.path, rec.Code, tt.status)
		}
		if got := rec.Header().Get("Content-Type"); !strings.Contains(got, tt.contentType) {
			t.Fatalf("%s Content-Type = %q, want it to contain %q", tt.path, got, tt.contentType)
		}
		if got := rec.Body.String(); !strings.Contains(got, tt.body) {
			t.Fatalf("%s body missing %q", tt.path, tt.body)
		}
	}
}
