package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/greatfirewall/operator/internal/profile"
	"sigs.k8s.io/yaml"
)

// MockAPI serves authenticated profile fixtures, reloading tokens and YAML for every request.
type MockAPI struct {
	TokenFile    string
	ProfilesFile string
}

// ServeHTTP authenticates before lookup so an invalid token can never revoke a profile.
func (m *MockAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet && (r.URL.Path == "/healthz" || r.URL.Path == "/readyz") {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Authenticate before lookup: a bad token must never be interpreted as a revocation.
	if !m.authenticate(w, r) {
		return
	}
	m.serveProfile(w, r)
}

// serveProfile validates the route and returns one atomically validated fixture.
func (m *MockAPI) serveProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/profiles/") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/v1/profiles/")
	if !profile.ValidName(name) {
		http.Error(w, "invalid profile name", http.StatusBadRequest)
		return
	}
	profiles, err := loadProfiles(m.ProfilesFile)
	if err != nil {
		http.Error(w, "profiles unavailable", http.StatusInternalServerError)
		return
	}
	profile, ok := profiles[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

// loadProfiles rejects a malformed fixture atomically instead of pretending profiles disappeared.
func loadProfiles(path string) (map[string]profile.Profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read profiles file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxDocumentBytes+1))
	if err != nil || len(data) > MaxDocumentBytes {
		return nil, errors.New("invalid profiles file size")
	}
	var document struct {
		Profiles map[string]profile.Profile `json:"profiles"`
	}
	if err = yaml.UnmarshalStrict(data, &document); err != nil || document.Profiles == nil {
		return nil, errors.New("invalid profiles YAML")
	}
	for name, documentProfile := range document.Profiles {
		valid, validationErr := profile.Validate(documentProfile, name)
		if validationErr != nil {
			return nil, validationErr
		}
		document.Profiles[name] = valid
	}
	return document.Profiles, nil
}
