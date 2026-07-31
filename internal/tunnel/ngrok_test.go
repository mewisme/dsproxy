package tunnel

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type fakeAgent struct {
	mu         sync.Mutex
	url        string
	endpoint   sdkEndpoint
	err        error
	disconnect int
}

func (a *fakeAgent) Listen(_ context.Context, endpointURL string) (sdkEndpoint, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.url = endpointURL
	return a.endpoint, a.err
}

func (a *fakeAgent) Disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.disconnect++
	return nil
}

type fakeEndpoint struct {
	net.Listener
	endpointURL *url.URL
	mu          sync.Mutex
	closed      int
}

func newFakeEndpoint(t *testing.T, rawURL string) *fakeEndpoint {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeEndpoint{Listener: listener, endpointURL: u}
}

func (e *fakeEndpoint) URL() *url.URL { return e.endpointURL }

func (e *fakeEndpoint) Close() error {
	e.mu.Lock()
	e.closed++
	e.mu.Unlock()
	return e.Listener.Close()
}

func withNewAgent(t *testing.T, f func(string) (sdkAgent, error)) {
	t.Helper()
	old := newAgent
	newAgent = f
	t.Cleanup(func() { newAgent = old })
}

func TestNgrokProviderPassesConfiguredURL(t *testing.T) {
	endpoint := newFakeEndpoint(t, "https://actual.ngrok.app")
	agent := &fakeAgent{endpoint: endpoint}
	withNewAgent(t, func(string) (sdkAgent, error) { return agent, nil })

	got, err := NewNgrokProvider().Listen(context.Background(), NewConfig("secret", "https://reserved.ngrok.app"))
	if err != nil {
		t.Fatal(err)
	}
	if agent.url != "https://reserved.ngrok.app" {
		t.Fatalf("URL passed to adapter = %q", agent.url)
	}
	if got.PublicURL() != "https://actual.ngrok.app" {
		t.Fatalf("PublicURL = %q", got.PublicURL())
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNgrokProviderRandomOmitsConfiguredURL(t *testing.T) {
	endpoint := newFakeEndpoint(t, "https://random.ngrok.app")
	agent := &fakeAgent{endpoint: endpoint}
	withNewAgent(t, func(string) (sdkAgent, error) { return agent, nil })

	got, err := NewNgrokProvider().Listen(context.Background(), NewConfig("secret", ""))
	if err != nil {
		t.Fatal(err)
	}
	if agent.url != "" {
		t.Fatalf("random endpoint received configured URL %q", agent.url)
	}
	_ = got.Close()
}

func TestNgrokEndpointCloseIsRepeatable(t *testing.T) {
	endpoint := newFakeEndpoint(t, "https://random.ngrok.app")
	agent := &fakeAgent{endpoint: endpoint}
	withNewAgent(t, func(string) (sdkAgent, error) { return agent, nil })

	got, err := NewNgrokProvider().Listen(context.Background(), NewConfig("secret", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
	if agent.disconnect != 1 || endpoint.closed != 1 {
		t.Fatalf("close calls: disconnect=%d endpoint=%d", agent.disconnect, endpoint.closed)
	}
}

func TestNgrokProviderDoesNotExposeTokenInErrors(t *testing.T) {
	const token = "very-secret-token"
	withNewAgent(t, func(string) (sdkAgent, error) { return nil, errors.New(token) })
	_, err := NewNgrokProvider().Listen(context.Background(), NewConfig(token, ""))
	if err == nil {
		t.Fatal("expected startup error")
	}
	if got := err.Error(); strings.Contains(got, token) {
		t.Fatalf("error exposed token: %q", got)
	}
}

func TestNgrokProviderCancellationDisconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	agent := &fakeAgent{err: context.Canceled}
	withNewAgent(t, func(string) (sdkAgent, error) { return agent, nil })
	_, err := NewNgrokProvider().Listen(ctx, NewConfig("secret", ""))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if agent.disconnect != 1 {
		t.Fatalf("disconnect calls = %d, want 1", agent.disconnect)
	}
}

func TestNgrokProviderCancellationAfterEndpointCreationClosesEndpoint(t *testing.T) {
	endpoint := newFakeEndpoint(t, "https://random.ngrok.app")
	agent := &fakeAgent{endpoint: endpoint}
	withNewAgent(t, func(string) (sdkAgent, error) { return agent, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewNgrokProvider().Listen(ctx, NewConfig("secret", ""))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if agent.disconnect != 1 || endpoint.closed != 1 {
		t.Fatalf("cleanup: disconnect=%d endpoint=%d", agent.disconnect, endpoint.closed)
	}
}
