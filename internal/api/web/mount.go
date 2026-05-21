package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/silhuzz/cexyrouter/internal/api"
)

//go:embed static/index.html static/app.css static/app.js static/cexyrouter-logo.svg static/favicon.svg
var staticFiles embed.FS

var _ api.MountFunc = Mount

func Mount(r chi.Router, _ api.Deps) {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("web static fs: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(staticRoot))
	serveStatic := func(w http.ResponseWriter, r *http.Request) {
		setStaticHeaders(w, r.URL.Path)
		fileServer.ServeHTTP(w, r)
	}

	r.Get("/", serveStatic)
	r.Get("/index.html", serveStatic)
	r.Get("/app.css", serveStatic)
	r.Get("/app.js", serveStatic)
	r.Get("/cexyrouter-logo.svg", serveStatic)
	r.Get("/favicon.svg", serveStatic)
}

func setStaticHeaders(w http.ResponseWriter, path string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if path == "/" || strings.HasSuffix(path, ".html") {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
}
