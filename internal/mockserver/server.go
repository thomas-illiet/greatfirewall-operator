// Package mockserver runs the authenticated fixture API with a bounded HTTPS lifecycle.
package mockserver

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/greatfirewall/operator/internal/api"
	"github.com/greatfirewall/operator/internal/config"
)

// Start binds IPv4 and runs HTTPS until cancellation, failing on invalid TLS configuration.
func Start(ctx context.Context, settings config.Mock) error {
	certificate, err := tls.LoadX509KeyPair(settings.CertificateFile, settings.KeyFile)
	if err != nil {
		return errors.New("cannot load mock TLS certificate")
	}
	listener, err := net.Listen("tcp4", settings.Address)
	if err != nil {
		return errors.New("cannot bind mock HTTPS listener")
	}
	defer listener.Close()
	server := &http.Server{
		Handler: &api.MockAPI{
			TokenFile:    settings.TokenFile,
			ProfilesFile: settings.ProfilesFile,
		},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16384,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
		},
	}
	slog.InfoContext(ctx, "Mock API listening", "address", listener.Addr().String())
	return serve(ctx, server, listener)
}

// serve propagates listener failures and gracefully drains requests on shutdown.
func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
