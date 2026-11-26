package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web
var webFiles embed.FS

type scenePayload struct {
	ImageURL    string        `json:"imageUrl"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Reference   referenceInfo `json:"reference"`
	Description string        `json:"description"`
}

type referenceInfo struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Width float64 `json:"width"`
	Label string  `json:"label"`
}

func main() {
	addr := flag.String("addr", envOr("EXPOSURE_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	flag.Parse()

	scene, info, err := buildSampleScene()
	if err != nil {
		log.Fatalf("build sample scene: %v", err)
	}
	web, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(web)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /api/scene", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(info)
	})
	mux.HandleFunc("GET /sample.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeContent(w, r, "sample.png", time.Time{}, bytes.NewReader(scene))
	})

	server := &http.Server{
		Addr:              *addr,
		Handler:           noCache(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("Exposure Lab listening on http://%s\n", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" || r.URL.Path == "/index.html":
			w.Header().Set("Cache-Control", "no-cache")
		case len(r.URL.Path) >= 4 && r.URL.Path[len(r.URL.Path)-4:] == ".png":
			// The scene endpoint supplies its own cache policy.
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}
