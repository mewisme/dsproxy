package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"dsproxy/internal/config"
	"dsproxy/internal/log"
	"dsproxy/internal/proxy"
	"dsproxy/internal/reasoning"
	"dsproxy/internal/tunnel"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runConfigured(ctx, cfg, tunnel.NewNgrokProvider, net.Listen)
}

type tunnelProviderFactory func() tunnel.Provider
type listenFunc func(network, address string) (net.Listener, error)

func runConfigured(ctx context.Context, cfg config.ProxyConfig, newProvider tunnelProviderFactory, listen listenFunc) int {
	log.Init(cfg.Verbose)
	log.Info("startup", "version", "dev", "host", cfg.Host, "port", cfg.Port)
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

	local, err := listen("tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if err != nil {
		log.Error("local listener", "err", err)
		return 1
	}

	var endpoint tunnel.Endpoint
	if cfg.NgrokEnabled {
		endpoint, err = newProvider().Listen(ctx, tunnel.NewConfig(cfg.NgrokAuthtoken, cfg.NgrokURL))
		if err != nil {
			_ = local.Close()
			if ctx.Err() != nil {
				return 0
			}
			log.Error("embedded ngrok startup", "err", err)
			return 1
		}
		defer endpoint.Close()
		log.Info("public_base_url", "url", strings.TrimRight(endpoint.PublicURL(), "/")+"/v1")
	}

	type serveResult struct {
		component string
		err       error
	}
	resultCount := 1
	errs := make(chan serveResult, 2)
	go func() { errs <- serveResult{"local", srv.Serve(local)} }()
	if endpoint != nil {
		resultCount++
		go func() { errs <- serveResult{"ngrok", srv.Serve(endpoint)} }()
	}

	shutdown := func(completed int) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		if endpoint != nil {
			_ = endpoint.Close()
		}
		for range resultCount - completed {
			<-errs
		}
	}

	select {
	case <-ctx.Done():
		shutdown(0)
		return 0
	case result := <-errs:
		if expectedServeError(result.err) {
			shutdown(1)
			return 0
		}
		log.Error("server error", "component", result.component, "err", result.err)
		shutdown(1)
		return 1
	}
}

func expectedServeError(err error) bool {
	return err == nil || errors.Is(err, http.ErrServerClosed)
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
