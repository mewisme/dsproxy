// Package tunnel defines the embedded public endpoint used by dsproxy.
package tunnel

import (
	"context"
	"net"
)

// Endpoint is a public listener supplied by a tunnel provider.
type Endpoint interface {
	net.Listener
	PublicURL() string
}

// Provider creates public endpoint listeners.
type Provider interface {
	Listen(context.Context, Config) (Endpoint, error)
}

// Config deliberately keeps the authtoken private so it cannot accidentally be
// formatted by callers. Construct it with NewConfig.
type Config struct {
	authtoken string
	url       string
}

func NewConfig(authtoken, url string) Config {
	return Config{authtoken: authtoken, url: url}
}
