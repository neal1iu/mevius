package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mevius/internal/api"
	"mevius/internal/catalog"
	"mevius/internal/config"
	"mevius/internal/integrations"
	cfprov "mevius/internal/provider/cloudflare"
	ghprov "mevius/internal/provider/github"
	vcprov "mevius/internal/provider/vercel"
	"mevius/internal/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset":
			os.Exit(resetRun(os.Args[2:]))
		case "seed":
			os.Exit(seedRun(os.Args[2:]))
		}
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mevius: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("store open: %w", err)
	}
	defer db.Close()

	var masterKey [32]byte
	copy(masterKey[:], cfg.MasterKey)

	reg := catalog.NewRegistry()
	directory := catalog.NewService(db, reg, masterKey)
	integrations.Register(directory, ghprov.NewProvider())
	integrations.Register(directory, cfprov.NewProvider())
	integrations.Register(directory, vcprov.NewProvider())
	if err := reg.ValidateReady(); err != nil {
		return err
	}
	if err := directory.RecoverInterrupted(context.Background()); err != nil {
		return err
	}
	oauth := catalog.NewOAuthService(directory, cfg.PublicURL, cfg.OAuthClients)
	if err := oauth.ExpireSessions(context.Background()); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewCatalogRouter(cfg.APIToken, logger, directory, oauth),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("server stopped")
	return nil
}
