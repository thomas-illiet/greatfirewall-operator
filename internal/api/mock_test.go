package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestMockAuthentication verifies authentication precedes lookup and applies to all data routes.
func TestMockAuthentication(t *testing.T) {
	dir := t.TempDir()
	token := writeFile(t, filepath.Join(dir, "tokens"), testToken)
	profiles := writeFile(t, filepath.Join(dir, "profiles.yaml"), "profiles:\n  dev: "+profileJSON+"\n")
	api := &MockAPI{TokenFile: token, ProfilesFile: profiles}
	cases := []struct {
		method, path, auth string
		want               int
	}{
		{"GET", "/healthz", "", 200},
		{"GET", "/readyz", "", 200},
		{"GET", "/v1/profiles/dev", "", 401},
		{"GET", "/v1/profiles/missing", "wrong", 401},
		{"GET", "/v1/profiles/dev", testToken, 200},
		{"GET", "/v1/profiles/missing", testToken, 404},
		{"POST", "/v1/profiles/dev", testToken, 405},
		{"GET", "/other", testToken, 404},
		{"GET", "/v1/profiles/invalid/name", testToken, 400},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.auth != "" {
			req.Header.Set("Authorization", "Bearer "+tc.auth)
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/v1/profiles/dev", nil)
	req.Header.Add("Authorization", "Bearer "+testToken)
	req.Header.Add("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("ambiguous headers accepted")
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	writeFile(t, profiles, "profiles: [")
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Fatal("invalid YAML incorrectly revoked profiles")
	}
	writeFile(t, token, "")
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("missing API token accepted")
	}
}

// TestLoadProfiles rejects invalid and oversized YAML atomically rather than dropping entries.
func TestLoadProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if _, err := loadProfiles(path); err == nil {
		t.Fatal("missing file accepted")
	}
	for _, data := range []string{
		"profiles: null",
		"profiles: {}\nunknown: 1",
		strings.Repeat("x", MaxDocumentBytes+1),
		"profiles:\n  dev: " + strings.Replace(profileJSON, "192.0.2.1", "::1", 1),
	} {
		writeFile(t, path, data)
		if _, err := loadProfiles(path); err == nil {
			t.Fatal("invalid fixture accepted")
		}
	}
	writeFile(t, path, "profiles: {}")
	if profiles, err := loadProfiles(path); err != nil || len(profiles) != 0 {
		t.Fatal("empty profile set rejected")
	}
}

// TestMockTokenRotation accepts both tokens during overlap and rejects the retired token.
func TestMockTokenRotation(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, filepath.Join(dir, "tokens"), testToken)
	profiles := writeFile(t, filepath.Join(dir, "profiles.yaml"), "profiles: {}\n")
	server := &MockAPI{TokenFile: path, ProfilesFile: profiles}
	newToken := strings.Repeat("z", 48)
	writeFile(t, path, testToken+"\n"+newToken)

	for _, token := range []string{testToken, newToken} {
		request := httptest.NewRequest("GET", "/v1/profiles/absent", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatal("overlapping rotation token was rejected")
		}
	}

	writeFile(t, path, newToken)
	request := httptest.NewRequest("GET", "/v1/profiles/absent", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatal("retired token remained valid")
	}
}
