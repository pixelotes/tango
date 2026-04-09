package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tango/internal/api"
	"tango/internal/config"
	"tango/internal/database"
	"tango/internal/engine"
	"tango/internal/trackerlist"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	port := flag.Int("port", 0, "override listen port")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *port > 0 {
		cfg.Port = *port
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	eng, err := engine.New(cfg, db)
	if err != nil {
		log.Fatalf("Failed to start engine: %v", err)
	}
	defer eng.Close()

	// Tracker list enrichment
	if cfg.Trackers.AutoAdd && cfg.Trackers.ListURL != "" {
		interval, err := time.ParseDuration(cfg.Trackers.UpdateInterval)
		if err != nil {
			log.Fatalf("Invalid trackers.update_interval: %v", err)
		}
		tl := trackerlist.New(cfg.Trackers.ListURL, interval)
		tl.Start()
		defer tl.Stop()
		eng.SetTrackerList(tl)
		log.Printf("Tracker list enrichment enabled (url=%s, interval=%s)", cfg.Trackers.ListURL, cfg.Trackers.UpdateInterval)
	}

	router := api.NewRouter(eng, cfg)

	srv := &http.Server{
		Addr:         cfg.ListenAddr(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Tango v%s listening on %s", api.Version, cfg.ListenAddr())
		log.Printf("aria2c RPC endpoint: %s", cfg.Aria2.RPCUrl)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	srv.Shutdown(ctx)
	log.Println("Tango stopped.")
}
