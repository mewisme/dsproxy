package tunnel

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sync"

	"golang.ngrok.com/ngrok/v2"
)

type sdkEndpoint interface {
	net.Listener
	URL() *url.URL
}

type sdkAgent interface {
	Listen(context.Context, string) (sdkEndpoint, error)
	Disconnect() error
}

var newAgent = func(token string) (sdkAgent, error) {
	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(token))
	if err != nil {
		return nil, err
	}
	return ngrokAgent{Agent: agent}, nil
}

type ngrokAgent struct{ ngrok.Agent }

func (a ngrokAgent) Listen(ctx context.Context, endpointURL string) (sdkEndpoint, error) {
	if endpointURL == "" {
		return a.Agent.Listen(ctx)
	}
	return a.Agent.Listen(ctx, ngrok.WithURL(endpointURL))
}

func (a ngrokAgent) Disconnect() error { return a.Agent.Disconnect() }

// NgrokProvider creates endpoints using the embedded ngrok Go SDK.
type NgrokProvider struct{}

func NewNgrokProvider() Provider { return NgrokProvider{} }

func (NgrokProvider) Listen(ctx context.Context, cfg Config) (Endpoint, error) {
	agent, err := newAgent(cfg.authtoken)
	if err != nil {
		return nil, safeError("initializing embedded ngrok agent", ctx)
	}

	listener, err := agent.Listen(ctx, cfg.url)
	if err != nil {
		_ = agent.Disconnect()
		return nil, safeError("creating embedded ngrok endpoint", ctx)
	}
	if err := ctx.Err(); err != nil {
		_ = listener.Close()
		_ = agent.Disconnect()
		return nil, err
	}
	return &ngrokEndpoint{sdkEndpoint: listener, agent: agent}, nil
}

func safeError(operation string, ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New(operation + ": ngrok SDK operation failed")
}

type ngrokEndpoint struct {
	sdkEndpoint
	agent     sdkAgent
	closeOnce sync.Once
	closeErr  error
}

func (e *ngrokEndpoint) PublicURL() string {
	if u := e.URL(); u != nil {
		return u.String()
	}
	return ""
}

func (e *ngrokEndpoint) Close() error {
	e.closeOnce.Do(func() {
		if err := e.sdkEndpoint.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			e.closeErr = errors.New("closing embedded ngrok endpoint failed")
		}
		if err := e.agent.Disconnect(); err != nil && e.closeErr == nil {
			e.closeErr = errors.New("disconnecting embedded ngrok agent failed")
		}
	})
	return e.closeErr
}
