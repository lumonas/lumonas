package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	listen := flag.String("listen", envOr("MYNAS_WEB_LISTEN", "127.0.0.1:8081"), "HTTP listen address")
	root := flag.String("root", envOr("MYNAS_WEB_ROOT", "/usr/share/mynas/web"), "compiled frontend root")
	api := flag.String("api", envOr("MYNAS_API_URL", "http://127.0.0.1:8080"), "backend URL")
	flag.Parse()
	target, err := url.Parse(*api)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	static := http.FileServer(http.Dir(*root))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			proxy.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(os.DirFS(*root), strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")); err != nil || r.URL.Path == "/" {
			r.URL.Path = "/index.html"
		}
		static.ServeHTTP(w, r)
	})
	log.Printf("mynas-web listening on %s, serving %s", *listen, *root)
	log.Fatal(http.ListenAndServe(*listen, handler))
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
