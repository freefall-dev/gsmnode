// Command server runs the gsmnode API Server. It is the single trusted
// entry point in front of PocketBase: the Web App and Phone Agent talk only to
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
	"smsgateway/apiserver/internal/bootstrap"
	"smsgateway/apiserver/internal/config"
	"smsgateway/apiserver/internal/pb"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[api] ")

	cfg := config.Load()

	client := pb.New(cfg.PocketBaseURL, cfg.PBAdminEmail, cfg.PBAdminPass)

	// Bring PocketBase up to the expected schema (create missing collections,
	// reconcile existing ones) and ensure the super-admin. Idempotent, so it runs
	// on every boot. Non-fatal: a fresh PocketBase that isn't reachable yet, or a
	// bad service account, must not stop the server from coming up so an operator
	// can fix the connection.
	if cfg.Bootstrap && cfg.AdminConfigured() {
		bootCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := bootstrap.Run(bootCtx, client, bootstrap.Options{
			UsersCollection:    "users",
			SuperAdminEmail:    cfg.SuperAdminEmail,
			SuperAdminPassword: cfg.SuperAdminPassword,
			SuperAdminName:     cfg.SuperAdminName,
		}); err != nil {
			log.Printf("WARNING: bootstrap failed: %v", err)
		} else {
			log.Println("bootstrap: PocketBase schema ready")
		}
		cancel()
	}

	srv := api.New(cfg, client)

	// Load persisted plugin state and start enabled plugins (e.g. the
	// email-to-sms SMTP server / IMAP poller). Non-fatal: a plugin that fails to
	// start must not stop the server coming up.
	if err := srv.StartPlugins(); err != nil {
		log.Printf("plugins: load failed: %v", err)
	}

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
	srv.StopPlugins(ctx)
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("stopped")
}
