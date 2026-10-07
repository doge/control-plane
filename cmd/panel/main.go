package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	panel "github.com/example/control-plane/internal/panel/controller"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	cfg := panel.Config{Addr: os.Getenv("PANEL_ADDR"), MongoURI: os.Getenv("MONGO_URI"), MongoDB: os.Getenv("MONGO_DB"), StaticDir: os.Getenv("STATIC_DIR"), TLSCertFile: os.Getenv("PANEL_TLS_CERT_FILE"), TLSKeyFile: os.Getenv("PANEL_TLS_KEY_FILE"), SessionTTL: durationEnv("SESSION_TTL_HOURS", 168) * time.Hour}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.MongoURI == "" {
		cfg.MongoURI = "mongodb://localhost:27017"
	}
	if cfg.MongoDB == "" {
		cfg.MongoDB = "control-plane"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "./web/dist"
	}
	app, err := panel.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close(context.Background())
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(cctx)
	}()
	log.Printf("Control Plane listening on %s", cfg.Addr)
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		log.Fatal("PANEL_TLS_CERT_FILE and PANEL_TLS_KEY_FILE must be configured together")
	}
	var serveErr error
	if cfg.TLSCertFile != "" {
		serveErr = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		serveErr = srv.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Fatal(serveErr)
	}
}
func durationEnv(key string, fallback int) time.Duration {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return time.Duration(n)
		}
	}
	return time.Duration(fallback)
}
