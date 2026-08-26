package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"foss-seeder/internal/config"
	"foss-seeder/internal/feed"
	"foss-seeder/internal/logger"
	"foss-seeder/internal/qbit"
	"foss-seeder/internal/syncer"
	"foss-seeder/internal/web"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("   FOSS Seeder — Distro Rotation Daemon & Web UI   ")
	fmt.Println("==================================================")

	log := logger.GetDefault()
	cfg := config.LoadConfig()

	log.Info("Loaded configuration. Feed: %s", cfg.FeedURL)
	log.Info("Configured qBittorrent Host: %s, Category: %s", cfg.QbitHost, cfg.QbitCategory)

	feedClient := feed.NewClient()
	qbitClient, err := qbit.NewClient(cfg.QbitHost, cfg.QbitUser, cfg.QbitPass)
	if err != nil {
		log.Error("Failed to initialize qBittorrent client: %v", err)
		os.Exit(1)
	}

	syncEngine := syncer.New(cfg, feedClient, qbitClient, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background sync loop
	syncEngine.Start(ctx)

	// Initialize Web Server
	srv, err := web.NewServer(cfg, syncEngine, qbitClient, log)
	if err != nil {
		log.Error("Failed to create web server: %v", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown listener
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("Web UI is listening on http://%s (Port: %s)", addr, cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("HTTP server error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-stopChan
	log.Info("Received signal %v. Initiating graceful shutdown...", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown: %v", err)
	}

	log.Info("FOSS Seeder stopped cleanly.")
}
