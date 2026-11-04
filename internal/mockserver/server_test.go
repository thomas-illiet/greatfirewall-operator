package mockserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/greatfirewall/operator/internal/config"
)

// tlsSettings extracts ephemeral test-only credentials from a local HTTPS server.
func tlsSettings(t *testing.T) config.Mock {
	t.Helper()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	certificate := server.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	settings := config.Mock{
		Address:         "127.0.0.1:0",
		CertificateFile: filepath.Join(dir, "cert.pem"),
		KeyFile:         filepath.Join(dir, "key.pem"),
	}
	files := map[string]*pem.Block{
		settings.CertificateFile: {Type: "CERTIFICATE", Bytes: certificate.Certificate[0]},
		settings.KeyFile:         {Type: "PRIVATE KEY", Bytes: key},
	}
	for path, block := range files {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return settings
}

// TestStart validates TLS startup, cancellation and occupied-listener failures.
func TestStart(t *testing.T) {
	if Start(context.Background(), config.Mock{}) == nil {
		t.Fatal("missing certificate accepted")
	}
	settings := tlsSettings(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Start(ctx, settings); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	settings.Address = listener.Addr().String()
	if Start(ctx, settings) == nil {
		t.Fatal("occupied port accepted")
	}
}

// TestServeErrors verifies startup errors reach the caller and closed servers stop cleanly.
func TestServeErrors(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if serve(context.Background(), &http.Server{}, listener) == nil {
		t.Fatal("invalid TLS configuration silently accepted")
	}
	server := &http.Server{
		TLSConfig: &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				return nil, nil
			},
		},
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := serve(context.Background(), server, listener); err != nil {
		t.Fatal(err)
	}
}
