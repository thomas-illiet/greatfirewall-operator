// Package config loads strict YAML and explicit environment overrides with Viper.
package config

import (
	"errors"
	"time"

	"github.com/greatfirewall/operator/internal/policy"
	"github.com/spf13/viper"
)

// Operator configures reconciliation and references secrets by mounted file path.
type Operator struct {
	APIURL          string        `mapstructure:"apiURL"`
	TokenFile       string        `mapstructure:"tokenFile"`
	CAFile          string        `mapstructure:"caFile"`
	Namespace       string        `mapstructure:"namespace"`
	RefreshInterval time.Duration `mapstructure:"refreshInterval"`
	HTTPTimeout     time.Duration `mapstructure:"httpTimeout"`
	HealthAddress   string        `mapstructure:"healthAddress"`
	MetricsAddress  string        `mapstructure:"metricsAddress"`
	LeaderElection  bool          `mapstructure:"leaderElection"`
	IPv6Disabled    bool          `mapstructure:"ipv6Disabled"`
	DNS             policy.DNS    `mapstructure:"dns"`
}

// LoadOperator applies defaults and explicit environment overrides, rejecting unknown YAML keys.
func LoadOperator(path string) (Operator, error) {
	v := operatorViper()
	var config Operator
	if err := read(v, path, &config); err != nil {
		return Operator{}, err
	}
	if v.InConfig("dns.selector") {
		// A custom selector replaces the default; merging would silently add constraints.
		config.DNS.Selector = v.GetStringMapString("dns.selector")
	}
	if config.Namespace == "" || !config.IPv6Disabled || config.RefreshInterval <= 0 || config.HTTPTimeout <= 0 {
		return Operator{}, errors.New("namespace, positive durations and ipv6Disabled acknowledgement required")
	}
	if err := policy.ValidateDNS(config.DNS); err != nil {
		return Operator{}, err
	}
	return config, nil
}

// Mock configures the HTTPS fixture server without storing secret values in YAML.
type Mock struct {
	Address         string `mapstructure:"address"`
	TokenFile       string `mapstructure:"tokenFile"`
	ProfilesFile    string `mapstructure:"profilesFile"`
	CertificateFile string `mapstructure:"certificateFile"`
	KeyFile         string `mapstructure:"keyFile"`
}

// LoadMock validates required fixture, token and TLS file references.
func LoadMock(path string) (Mock, error) {
	v := viper.New()
	v.SetDefault("address", ":8443")
	var config Mock
	if err := read(v, path, &config); err != nil {
		return Mock{}, err
	}
	if config.TokenFile == "" || config.ProfilesFile == "" || config.CertificateFile == "" || config.KeyFile == "" {
		return Mock{}, errors.New("mock token, profiles and TLS files required")
	}
	return config, nil
}

// read loads YAML into a typed configuration and sanitizes decoder errors.
func read(v *viper.Viper, path string, target any) error {
	if path == "" {
		return errors.New("configuration path is required")
	}
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return errors.New("cannot read configuration YAML")
	}
	if err := v.UnmarshalExact(target); err != nil {
		return errors.New("invalid configuration fields or types")
	}
	return nil
}
