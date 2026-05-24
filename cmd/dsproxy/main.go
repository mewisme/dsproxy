package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dsproxy/internal/config"
	"dsproxy/internal/proxy"
	"dsproxy/internal/reasoning"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		return 2
	}

	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	slog.Info("loaded config", "env", cfg.EnvPath)
	warnInsecureUpstream(cfg.UpstreamBaseURL)

	store, err := reasoning.Open(cfg.ReasoningContentPath, cfg.ReasoningCacheMaxAgeSeconds, cfg.ReasoningCacheMaxRows)
	if err != nil {
		slog.Error("reasoning store", "err", err)
		return 2
	}
	defer store.Close()

	if cfg.ClearReasoningCache {
		n, err := store.Clear()
		if err != nil {
			slog.Error("clear cache", "err", err)
			return 2
		}
		slog.Info("cleared reasoning cache", "rows", n)
		return 0
	}

	srv := proxy.NewServer(cfg, store)
	slog.Info("default_model", "model", cfg.UpstreamModel, "thinking", cfg.Thinking, "effort", cfg.ReasoningEffort)
	slog.Info("local_base_url", "url", proxy.LocalBaseURL(cfg.Host, cfg.Port))
	for _, u := range proxy.ExposedURLs(cfg.Host, cfg.Port) {
		slog.Info("exposed_base_url", "url", u)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return 0
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			return 1
		}
		return 0
	}
}

func warnInsecureUpstream(baseURL string) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" {
		return
	}
	host := u.Hostname()
	if host == "127.0.0.1" || host == "localhost" || host == "::1" {
		return
	}
	slog.Warn("upstream base_url uses plain HTTP; bearer tokens may be exposed")
}
