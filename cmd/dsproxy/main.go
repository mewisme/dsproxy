package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dsproxy/internal/config"
	"dsproxy/internal/log"
	"dsproxy/internal/proxy"
	"dsproxy/internal/reasoning"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		log.Error("config error", "err", err)
		return 2
	}

	if err := cfg.Validate(); err != nil {
		log.Error("config validation", "err", err)
		return 2
	}

	log.Init(cfg.Verbose)

	log.Info("startup",
		"version", "dev",
		"host", cfg.Host,
		"port", cfg.Port,
	)
	if len(cfg.LoadedEnvFiles) > 0 {
		log.Info("loaded env files", "files", cfg.LoadedEnvFiles)
	}
	log.Info("\n" + cfg.StartupSummary())

	warnInsecureUpstream(cfg.UpstreamBaseURL)

	store, err := reasoning.Open(cfg.ReasoningContentPath, cfg.ReasoningCacheMaxAgeSeconds, cfg.ReasoningCacheMaxRows)
	if err != nil {
		log.Error("reasoning store", "err", err)
		return 2
	}
	defer store.Close()

	if cfg.ClearReasoningCache {
		n, err := store.Clear()
		if err != nil {
			log.Error("clear cache", "err", err)
			return 2
		}
		log.Info("cleared reasoning cache", "rows", n)
		return 0
	}

	srv := proxy.NewServer(cfg, store)
	log.Info("default_model", "model", cfg.UpstreamModel, "thinking", cfg.Thinking, "effort", cfg.ReasoningEffort)
	log.Info("local_base_url", "url", proxy.LocalBaseURL(cfg.Host, cfg.Port))
	for _, u := range proxy.ExposedURLs(cfg.Host, cfg.Port) {
		log.Info("exposed_base_url", "url", u)
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
			log.Error("server error", "err", err)
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
	if host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "0.0.0.0" {
		return
	}
	log.Warn("upstream base_url uses plain HTTP; bearer tokens may be exposed")
}
