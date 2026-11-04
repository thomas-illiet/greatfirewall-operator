package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// configFile writes one isolated configuration fixture for a test case.
func configFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestOperatorDefaults checks defaults, environment precedence and DNS validation.
func TestOperatorDefaults(t *testing.T) {
	path := configFile(t, "namespace: system\nipv6Disabled: true\n")
	settings, err := LoadOperator(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.RefreshInterval != time.Minute || settings.HTTPTimeout != 5*time.Second {
		t.Fatal("unexpected default intervals")
	}
	if !settings.LeaderElection || settings.DNS.Selector["k8s-app"] != "kube-dns" {
		t.Fatal("security defaults missing")
	}

	t.Setenv("GFW_REFRESH_INTERVAL", "2m")
	t.Setenv("GFW_API_URL", "https://profiles.example.test")
	settings, err = LoadOperator(path)
	if err != nil || settings.RefreshInterval != 2*time.Minute || settings.APIURL == "" {
		t.Fatal("environment overrides were not applied")
	}
}

// TestInvalidOperatorConfiguration rejects ambiguous configuration and unsupported networking.
func TestInvalidOperatorConfiguration(t *testing.T) {
	cases := []string{
		"namespace: system\nipv6Disabled: false\n",
		"ipv6Disabled: true\n",
		"namespace: system\nipv6Disabled: true\nrefreshInterval: 0s\n",
		"namespace: system\nipv6Disabled: true\nhttpTimeout: wrong\n",
		"namespace: system\nipv6Disabled: true\nunknown: value\n",
		"namespace: system\nipv6Disabled: true\ndns:\n  addresses: ['::1']\n",
		"invalid: [",
	}
	for _, text := range cases {
		if _, err := LoadOperator(configFile(t, text)); err == nil {
			t.Errorf("invalid configuration accepted: %q", text)
		}
	}
	for _, path := range []string{"", "missing.yaml"} {
		if _, err := LoadOperator(path); err == nil {
			t.Fatal("missing configuration accepted")
		}
	}
}

// TestMockConfiguration requires all secret and fixture references, with strict YAML fields.
func TestMockConfiguration(t *testing.T) {
	text := "tokenFile: /token\nprofilesFile: /profiles\ncertificateFile: /cert\nkeyFile: /key\n"
	settings, err := LoadMock(configFile(t, text))
	if err != nil || settings.Address != ":8443" {
		t.Fatalf("valid mock configuration failed: %v", err)
	}
	for _, text := range []string{"{}", "unknown: value", "address: ["} {
		if _, err := LoadMock(configFile(t, text)); err == nil {
			t.Fatal("invalid mock configuration accepted")
		}
	}
}

// TestDNSSelectorReplacement avoids adding default labels to custom resolver selectors.
func TestDNSSelectorReplacement(t *testing.T) {
	path := configFile(t, "namespace: system\nipv6Disabled: true\ndns:\n  selector:\n    app: coredns\n")
	settings, err := LoadOperator(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.DNS.Selector) != 1 || settings.DNS.Selector["app"] != "coredns" {
		t.Fatalf("default selector leaked into custom configuration: %v", settings.DNS.Selector)
	}
}
