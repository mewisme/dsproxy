package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"dsproxy/internal/log"

	"dsproxy/internal/config"
	"dsproxy/internal/reasoning"
)

type Server struct {
	Config         config.ProxyConfig
	ReasoningStore *reasoning.Store
	HTTP           *http.Server
}

func NewServer(cfg config.ProxyConfig, store *reasoning.Store) *Server {
	s := &Server{Config: cfg, ReasoningStore: store}
	handler := &Handler{
		Config: cfg,
		Store:  store,
		Client: &http.Client{Timeout: time.Duration(cfg.RequestTimeout * float64(time.Second))},
	}
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	s.HTTP = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       time.Duration(cfg.RequestTimeout * float64(time.Second)),
		// WriteTimeout must be 0 to prevent killing long SSE streams.
		// Timeout enforcement relies on http.Client.Timeout and upstream context.
		WriteTimeout: 0,
	}
	return s
}

func (s *Server) ListenAndServe() error {
	log.Info("listening", "addr", s.HTTP.Addr, "base_url", fmt.Sprintf("http://%s/v1", s.HTTP.Addr))
	return s.HTTP.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.HTTP.Shutdown(ctx)
}

func LocalBaseURL(host string, port int) string {
	return fmt.Sprintf("http://%s:%d/v1", host, port)
}

func ExposedURLs(host string, port int) []string {
	if host != "0.0.0.0" && host != "::" {
		return nil
	}
	var urls []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return urls
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			urls = append(urls, fmt.Sprintf("http://%s:%d/v1", ip.String(), port))
		}
	}
	return urls
}
