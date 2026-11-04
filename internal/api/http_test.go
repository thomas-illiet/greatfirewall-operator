package api

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef"
const profileJSON = `{"profile":"dev","revision":"1","egress":[{"cidr":"192.0.2.1","ports":[{"protocol":"TCP","port":443}]}]}`

// reply describes the HTTP response served by the current test scenario.
type reply struct {
	status int
	body   string
}

// writeFile creates a private test fixture without logging its contents.
func writeFile(t *testing.T, path, value string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// trustedClient builds a client trusting only the local TLS server certificate.
func trustedClient(t *testing.T, server *httptest.Server, token string) *APIClient {
	t.Helper()
	dir := t.TempDir()
	ca := writeFile(t, filepath.Join(dir, "ca.pem"), string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})))
	client, err := NewAPIClient(server.URL, writeFile(t, filepath.Join(dir, "token"), token), ca, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// TestAPIClient exercises TLS, authentication, rotation, status handling and strict JSON parsing.
func TestAPIClient(t *testing.T) {
	var current atomic.Value
	current.Store(reply{200, profileJSON})
	var authorization atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization.Store(r.Header.Get("Authorization"))
		value := current.Load().(reply)
		w.Header().Set("Location", "https://example.invalid/")
		w.WriteHeader(value.status)
		_, _ = io.WriteString(w, value.body)
	}))
	defer server.Close()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	client := trustedClient(t, server, testToken)

	p, err := client.Fetch(context.Background(), "dev")
	if err != nil || p.Egress[0].CIDR != "192.0.2.1/32" {
		t.Fatalf("valid fetch failed: %v", err)
	}

	// Each request must read the new token from the projected Secret file.
	newToken := strings.Repeat("z", 40)
	writeFile(t, client.tokenFile, newToken)
	_, err = client.Fetch(context.Background(), "dev")
	if err != nil || authorization.Load() != "Bearer "+newToken {
		t.Fatal("token rotation failed")
	}
	checkRejectedReplies(t, client, &current)
	checkClientFailures(t, client, server)
}

// checkRejectedReplies verifies complete validation and prevents secrets escaping in errors.
func checkRejectedReplies(t *testing.T, client *APIClient, current *atomic.Value) {
	t.Helper()
	for _, value := range []reply{
		{404, testToken},
		{401, testToken},
		{403, testToken},
		{500, testToken},
		{302, testToken},
		{200, "{"},
		{200, profileJSON + " {}"},
		{200, strings.Replace(profileJSON, "192.0.2.1", "::1", 1)},
		{200, strings.Repeat("x", MaxDocumentBytes+1)},
		{
			200,
			strings.Replace(profileJSON, `"revision":"1"`, `"revision":"1","unknown":1`, 1),
		},
	} {
		current.Store(value)
		_, err := client.Fetch(context.Background(), "dev")
		if err == nil || strings.Contains(err.Error(), testToken) {
			t.Fatalf("unsafe status handling: %d %v", value.status, err)
		}
		if errors.Is(err, ErrRevoked) != (value.status == 404) {
			t.Fatal("incorrect revocation status")
		}
	}
}

// checkClientFailures verifies request prerequisites and mandatory TLS trust.
func checkClientFailures(t *testing.T, client *APIClient, server *httptest.Server) {
	t.Helper()
	if _, err := client.Fetch(context.Background(), "bad/name"); err == nil {
		t.Fatal("invalid name")
	}
	if _, err := client.Fetch(nil, "dev"); err == nil {
		t.Fatal("nil context")
	}

	writeFile(t, client.tokenFile, "")
	if _, err := client.Fetch(context.Background(), "dev"); err == nil {
		t.Fatal("missing token accepted")
	}

	untrusted, err := NewAPIClient(server.URL, writeFile(t, client.tokenFile, testToken), "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = untrusted.Fetch(context.Background(), "dev"); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
}

// TestClientConfiguration rejects insecure URLs, bad CA files and malformed token material.
func TestClientConfiguration(t *testing.T) {
	dir := t.TempDir()
	token := writeFile(t, filepath.Join(dir, "token"), testToken)
	for _, base := range []string{
		"http://example.com",
		"https://user:pass@example.com",
		"https://example.com?q=secret",
		"https://example.com#secret",
		"https://[::1]",
		"https://[::ffff:192.0.2.1]",
		"https://",
		"%",
	} {
		if _, err := NewAPIClient(base, token, "", time.Second); err == nil {
			t.Fatal("URL accepted")
		}
	}
	if _, err := NewAPIClient("https://example.com", token, "", 0); err == nil {
		t.Fatal("zero timeout")
	}
	if _, err := NewAPIClient("https://example.com", "missing", "", time.Second); err == nil {
		t.Fatal("missing token")
	}
	for _, ca := range []string{
		"missing",
		writeFile(t, filepath.Join(dir, "bad.pem"), "not a certificate"),
	} {
		if _, err := NewAPIClient("https://example.com", token, ca, time.Second); err == nil {
			t.Fatal("invalid CA")
		}
	}
	for _, value := range []string{
		"",
		"short",
		strings.Repeat("x", 513),
		strings.Repeat("x", 8193),
		testToken + ":",
		strings.Repeat(testToken+"\n", 9),
	} {
		writeFile(t, token, value)
		if _, err := readTokens(token, true); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	writeFile(t, token, testToken+"\n"+strings.Repeat("y", 40))
	if tokens, err := readTokens(token, true); err != nil || len(tokens) != 2 {
		t.Fatal("rotation set rejected")
	}
	if _, err := readTokens(token, false); err == nil {
		t.Fatal("client accepted multiple tokens")
	}
}
