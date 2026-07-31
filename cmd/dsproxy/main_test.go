package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"dsproxy/internal/config"
	"dsproxy/internal/tunnel"
)

type fakeProvider struct {
	endpoint tunnel.Endpoint
	err      error
	called   int
	ready    chan struct{}
}

func (p *fakeProvider) Listen(context.Context, tunnel.Config) (tunnel.Endpoint, error) {
	p.called++
	if p.ready != nil {
		close(p.ready)
	}
	return p.endpoint, p.err
}

type testEndpoint struct {
	net.Listener
	url    string
	mu     sync.Mutex
	closed int
}

func (e *testEndpoint) PublicURL() string { return e.url }

func (e *testEndpoint) Close() error {
	e.mu.Lock()
	e.closed++
	e.mu.Unlock()
	return e.Listener.Close()
}

func testConfig(t *testing.T, enabled bool) config.ProxyConfig {
	t.Helper()
	return config.ProxyConfig{
		Host: "127.0.0.1", Port: 9999, UpstreamBaseURL: config.DefaultUpstreamBaseURL,
		UpstreamModel: config.DefaultUpstreamModel, Thinking: config.DefaultThinking,
		ReasoningEffort: config.DefaultReasoningEffort, RequestTimeout: config.DefaultRequestTimeout,
		MaxRequestBodyBytes:  config.DefaultMaxRequestBodyBytes,
		ReasoningContentPath: t.TempDir() + "/reasoning.db", NgrokEnabled: enabled,
		NgrokAuthtoken: "test-token",
	}
}

func TestRunConfiguredDisabledDoesNotInstantiateProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var providerCreated bool
	code := runConfigured(ctx, testConfig(t, false), func() tunnel.Provider {
		providerCreated = true
		return &fakeProvider{}
	}, func(network, address string) (net.Listener, error) {
		return net.Listen(network, "127.0.0.1:0")
	})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if providerCreated {
		t.Fatal("disabled ngrok instantiated provider")
	}
}

func TestRunConfiguredNgrokFailurePreventsServing(t *testing.T) {
	cfg := testConfig(t, true)
	provider := &fakeProvider{err: errors.New("endpoint unavailable")}
	var local net.Listener
	code := runConfigured(context.Background(), cfg, func() tunnel.Provider { return provider }, func(network, address string) (net.Listener, error) {
		listener, err := net.Listen(network, "127.0.0.1:0")
		local = listener
		return listener, err
	})
	if code != 1 || provider.called != 1 {
		t.Fatalf("exit=%d provider calls=%d", code, provider.called)
	}
	if local == nil {
		t.Fatal("local listener was not created")
	}
	if _, err := local.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("local listener must be closed after ngrok startup failure, got %v", err)
	}
}

func TestRunConfiguredServesLocalAndPublicListenerThenShutsDown(t *testing.T) {
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	public, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &testEndpoint{Listener: public, url: "https://public.ngrok.app"}
	provider := &fakeProvider{endpoint: endpoint, ready: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan int, 1)
	go func() {
		done <- runConfigured(ctx, testConfig(t, true), func() tunnel.Provider { return provider }, func(string, string) (net.Listener, error) {
			return local, nil
		})
	}()
	<-provider.ready

	client := &http.Client{Timeout: time.Second}
	for _, listener := range []net.Listener{local, public} {
		resp, err := client.Get("http://" + listener.Addr().String() + "/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("health status = %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("runConfigured did not shut down")
	}
	endpoint.mu.Lock()
	closed := endpoint.closed
	endpoint.mu.Unlock()
	if closed == 0 {
		t.Fatal("public endpoint was not closed")
	}
}
