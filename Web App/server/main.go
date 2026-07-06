// Command webapp is the Web App backend-for-frontend. It serves the embedded
// Vue single-page app and reverse-proxies /api/* to the API Server, so the
// browser only ever talks to this server (same-origin) and all data access
// still flows through the API Server.
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[web] ")

	loadDotEnv(".env")
	addr := getenv("WEB_ADDR", ":8090")
	apiBase := strings.TrimRight(getenv("API_BASE", "http://localhost:8080"), "/")

	apiURL, err := url.Parse(apiBase)
	if err != nil {
		log.Fatalf("invalid API_BASE %q: %v", apiBase, err)
	}

	// Reverse proxy: /api/* -> API Server (path preserved).
	proxy := httputil.NewSingleHostReverseProxy(apiURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		log.Printf("proxy error for %s: %v", r.URL.Path, e)
		http.Error(w, `{"error":"api server unavailable"}`, http.StatusBadGateway)
	}

	// Embedded SPA file server.
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatalf("embed dist: %v", err)
	}
	spa := http.FileServer(http.FS(sub))

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve static assets when they exist; otherwise fall back to index.html
		// so client-side routing works on deep links.
		if r.URL.Path != "/" {
			if f, err := sub.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
				f.Close()
				spa.ServeHTTP(w, r)
				return
			}
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		spa.ServeHTTP(w, r2)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s (proxying /api -> %s)", addr, apiBase)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// loadDotEnv loads KEY=VALUE pairs from a .env file if present.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
}
