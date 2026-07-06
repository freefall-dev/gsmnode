// Command server runs the gsmnode API Server. It is the single trusted
// entry point in front of PocketBase: the Web App and Phone App talk only to
// this service, which in turn performs all PocketBase access.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smsgateway/apiserver/internal/api"
	"smsgateway/apiserver/internal/auth"
	"smsgateway/apiserver/internal/config"
	"smsgateway/apiserver/internal/pb"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[api] ")

	cfg := config.Load()

	client := pb.New(cfg.PocketBaseURL, cfg.PBAdminEmail, cfg.PBAdminPass)
	jwtMgr := auth.NewManager(cfg.JWTSecret, cfg.JWTAccessTTL)
	srv := api.New(cfg, client, jwtMgr)

	// Background worker that fails messages no device processed in time.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	srv.StartExpiryWorker(workerCtx)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (PocketBase: %s)", cfg.Addr, cfg.PocketBaseURL)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("stopped")
}
