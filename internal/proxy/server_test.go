package proxy

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"dsproxy/internal/config"
	"dsproxy/internal/reasoning"
)

func TestServerServeUsesSameHandlerForMultipleListeners(t *testing.T) {
	store, err := reasoning.Open(t.TempDir()+"/reasoning.db", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.ProxyConfig{
		Host: "127.0.0.1", Port: 9999, UpstreamBaseURL: config.DefaultUpstreamBaseURL,
		UpstreamModel: config.DefaultUpstreamModel, Thinking: config.DefaultThinking,
		ReasoningEffort: config.DefaultReasoningEffort, RequestTimeout: config.DefaultRequestTimeout,
		MaxRequestBodyBytes: config.DefaultMaxRequestBodyBytes,
	}
	srv := NewServer(cfg, store)
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	go func() { errs <- srv.Serve(first) }()
	go func() { errs <- srv.Serve(second) }()

	client := &http.Client{Timeout: time.Second}
	for _, listener := range []net.Listener{first, second} {
		resp, err := client.Get("http://" + listener.Addr().String() + "/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		_ = resp.Body.Close()

		resp, err = client.Post("http://"+listener.Addr().String()+"/v1/chat/completions", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("missing authorization status = %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-errs; err != http.ErrServerClosed {
			t.Fatalf("Serve returned %v", err)
		}
	}
}
