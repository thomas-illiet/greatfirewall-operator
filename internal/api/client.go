// Package api implements mandatory HTTPS and bearer authentication for profile exchange.
package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/greatfirewall/operator/internal/profile"
)

const MaxDocumentBytes = 512 * 1024

// ErrRevoked identifies an authenticated API response explicitly revoking a profile.
var ErrRevoked = errors.New("profile revoked (HTTP 404)")

// Fetcher obtains one validated profile or a sanitized error.
type Fetcher interface {
	// Fetch requests the named profile; ErrRevoked is reserved for HTTP 404.
	Fetch(context.Context, string) (profile.Profile, error)
}

// APIClient performs authenticated HTTPS requests and reloads the token on each request.
type APIClient struct {
	base      string
	tokenFile string
	http      *http.Client
}

// NewAPIClient validates HTTPS configuration and builds a client with mandatory TLS checks.
func NewAPIClient(base, tokenFile, caFile string, timeout time.Duration) (*APIClient, error) {
	u, err := url.Parse(base)
	if err != nil || !validBaseURL(u) || timeout <= 0 {
		return nil, errors.New("API URL must be HTTPS without credentials, query or fragment; positive timeout required")
	}
	if _, err = readTokens(tokenFile, false); err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		pem, readErr := os.ReadFile(caFile)
		if readErr != nil {
			return nil, errors.New("cannot read API CA file")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid API CA certificate")
		}
		tlsConfig.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	transport.DialContext = dialIPv4

	return &APIClient{
		base:      strings.TrimRight(base, "/"),
		tokenFile: tokenFile,
		http:      &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: rejectRedirect},
	}, nil
}

// validBaseURL disallows credential-bearing URLs and literal IPv6 service addresses.
func validBaseURL(address *url.URL) bool {
	if address.Scheme != "https" || address.Hostname() == "" || address.User != nil {
		return false
	}
	if address.RawQuery != "" || address.Fragment != "" {
		return false
	}
	ip, err := netip.ParseAddr(address.Hostname())
	return err != nil || ip.Is4()
}

// dialIPv4 keeps API connections on IPv4 even when DNS returns both address families.
func dialIPv4(ctx context.Context, _ string, address string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, "tcp4", address)
}

// rejectRedirect prevents credentials and revocation semantics from crossing redirects.
func rejectRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

// Fetch validates the complete response and never includes server bodies or credentials in errors.
func (c *APIClient) Fetch(ctx context.Context, name string) (profile.Profile, error) {
	if !profile.ValidName(name) {
		return profile.Profile{}, errors.New("invalid profile name")
	}

	// Re-read the mounted Secret so token rotation does not require a restart.
	tokens, err := readTokens(c.tokenFile, false)
	if err != nil {
		return profile.Profile{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/profiles/"+url.PathEscape(name), nil)
	if err != nil {
		return profile.Profile{}, errors.New("cannot create API request")
	}
	req.Header.Set("Authorization", "Bearer "+tokens[0])
	req.Header.Set("Accept", "application/json")

	response, err := c.http.Do(req)
	if err != nil {
		return profile.Profile{}, errors.New("API transport failure")
	}
	defer response.Body.Close()

	// Only HTTPS 404 responses revoke; authentication and transport failures retain state.
	if response.StatusCode == http.StatusNotFound {
		return profile.Profile{}, ErrRevoked
	}
	if response.StatusCode != http.StatusOK {
		return profile.Profile{}, fmt.Errorf("API HTTP status %d", response.StatusCode)
	}

	return decodeProfile(response.Body, name)
}

// decodeProfile bounds parsing and rejects partial, unknown or trailing JSON data.
func decodeProfile(reader io.Reader, name string) (profile.Profile, error) {
	body, err := io.ReadAll(io.LimitReader(reader, MaxDocumentBytes+1))
	if err != nil || len(body) > MaxDocumentBytes {
		return profile.Profile{}, errors.New("invalid API response size or read failure")
	}

	var document profile.Profile
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&document); err != nil {
		return profile.Profile{}, errors.New("invalid API JSON")
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return profile.Profile{}, errors.New("trailing API JSON data")
	}
	return profile.Validate(document, name)
}
