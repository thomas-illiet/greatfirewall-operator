package config

import "github.com/spf13/viper"

// operatorViper declares supported configuration keys and explicit environment overrides.
func operatorViper() *viper.Viper {
	v := viper.New()
	defaults := map[string]any{
		"apiURL":          "",
		"tokenFile":       "",
		"caFile":          "",
		"namespace":       "",
		"refreshInterval": "60s",
		"httpTimeout":     "5s",
		"healthAddress":   ":8081",
		"metricsAddress":  "0",
		"leaderElection":  true,
		"ipv6Disabled":    false,
		"dns.namespace":   "kube-system",
		"dns.selector":    map[string]string{"k8s-app": "kube-dns"},
		"dns.addresses":   []string{},
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}
	environment := map[string]string{
		"apiURL":          "GFW_API_URL",
		"tokenFile":       "GFW_TOKEN_FILE",
		"caFile":          "GFW_CA_FILE",
		"namespace":       "GFW_NAMESPACE",
		"refreshInterval": "GFW_REFRESH_INTERVAL",
		"httpTimeout":     "GFW_HTTP_TIMEOUT",
		"ipv6Disabled":    "GFW_IPV6_DISABLED",
	}
	for key, env := range environment {
		_ = v.BindEnv(key, env)
	}
	return v
}
