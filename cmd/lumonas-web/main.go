package main

import (
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	listen := flag.String("listen", envOr("LUMONAS_WEB_LISTEN", "127.0.0.1:8081"), "HTTP listen address")
	root := flag.String("root", envOr("LUMONAS_WEB_ROOT", "/usr/share/lumonas/web"), "compiled frontend root")
	api := flag.String("api", envOr("LUMONAS_API_URL", "http://127.0.0.1:8080"), "backend URL")
	cert := flag.String("tls-cert", envOr("LUMONAS_WEB_TLS_CERT", ""), "optional TLS certificate")
	key := flag.String("tls-key", envOr("LUMONAS_WEB_TLS_KEY", ""), "optional TLS private key")
	flag.Parse()
	target, err := url.Parse(*api)
	if err != nil {
		logger.Error("invalid backend URL", "error", err)
		os.Exit(1)
	}
	handler := newHandler(*root, target)
	if (*cert == "") != (*key == "") {
		logger.Error("both TLS certificate and key are required for HTTPS")
		os.Exit(1)
	}
	logger.Info("lumonas-web listening", "listen", *listen, "root", *root, "https", *cert != "")
	var serveErr error
	if *cert != "" {
		serveErr = http.ListenAndServeTLS(*listen, *cert, *key, handler)
	} else {
		serveErr = http.ListenAndServe(*listen, handler)
	}
	logger.Error("lumonas-web stopped", "error", serveErr)
	os.Exit(1)
}

func newHandler(root string, target *url.URL) http.Handler {
	return newHandlerWithTransport(root, target, http.DefaultTransport)
}

func newHandlerWithTransport(root string, target *url.URL, transport http.RoundTripper) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = transport
	static := http.FileServer(http.Dir(root))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			proxy.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(os.DirFS(root), strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")); err != nil || r.URL.Path == "/" {
			// Let FileServer resolve the directory index. Rewriting to
			// /index.html causes its canonical redirect to ./, which breaks
			// client-side routes such as /dashboard.
			r.URL.Path = "/"
			r.URL.RawPath = ""
		}
		static.ServeHTTP(w, r)
	})
	return handler
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
