package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

// readTokens loads a bounded token file; one token per line permits API-side rotation.
func readTokens(path string, multiple bool) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read token file")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(data) > 8192 {
		return nil, errors.New("invalid token file")
	}

	tokens := strings.Fields(string(data))
	if len(tokens) == 0 || (!multiple && len(tokens) != 1) || len(tokens) > 8 {
		return nil, errors.New("invalid token count")
	}

	for _, token := range tokens {
		if len(token) < 32 || len(token) > 512 {
			return nil, errors.New("token must contain 32 to 512 characters")
		}
		for _, char := range token {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("-._~+/=", char)) {
				return nil, errors.New("invalid token characters")
			}
		}
	}
	return tokens, nil
}

// authenticate reloads the accepted token set and rejects ambiguous authorization headers.
func (m *MockAPI) authenticate(w http.ResponseWriter, r *http.Request) bool {
	tokens, err := readTokens(m.TokenFile, true)
	if err != nil {
		http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
		return false
	}

	if !matchesToken(r.Header.Values("Authorization"), tokens) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

// matchesToken compares fixed-length digests without revealing which rotation token matched.
func matchesToken(headers, tokens []string) bool {
	if len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") {
		return false
	}

	digest := sha256.Sum256([]byte(strings.TrimPrefix(headers[0], "Bearer ")))
	matched := 0
	for _, token := range tokens {
		expected := sha256.Sum256([]byte(token))
		matched |= subtle.ConstantTimeCompare(digest[:], expected[:])
	}
	return matched == 1
}
