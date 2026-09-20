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
	"mevius/internal/config"
	"mevius/internal/provider"
	cfprov "mevius/internal/provider/cloudflare"
	ghprov "mevius/internal/provider/github"
	vcprov "mevius/internal/provider/vercel"
	"mevius/internal/service"
	"mevius/internal/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
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

	q := store.New(db)

	reg := provider.NewRegistry()
	reg.Register(ghprov.NewProvider())
	reg.Register(cfprov.NewProvider())
	reg.Register(vcprov.NewProvider())

	credStore := provider.NewCredentialStore(masterKey, q)

	connSvc := service.NewConnectionService(q, masterKey, reg)
	oauthSvc := service.NewOAuthService(q, masterKey, reg, connSvc, cfg.PublicURL, cfg.OAuthClients)
	projectSvc := service.NewProjectService(q)
	resourceSvc := service.NewResourceService(q, reg, credStore)
	linkSvc := service.NewLinkService(q)
	runtimeSvc := service.NewRuntimeService(resourceSvc)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewRouter(cfg.APIToken, logger, connSvc, projectSvc, resourceSvc, linkSvc, runtimeSvc, oauthSvc, reg),
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
