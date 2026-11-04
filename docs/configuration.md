# Configuration

## Cobra commands

```sh
operator --config /config/operator.yaml --log-level info
mock-api --config /config/mock.yaml --log-level info
```

`--config` is required. `--log-level` accepts `debug`, `info`, `warn`, and `error`. `--help` describes the options. Viper reads the YAML files; unknown keys and invalid types cause startup to fail.

## Operator

```yaml
apiURL: https://profiles.example.internal
tokenFile: /secrets/token/token
caFile: /secrets/ca/ca.crt
namespace: greatfirewall-system
ipv6Disabled: true
refreshInterval: 60s
httpTimeout: 5s
healthAddress: ":8081"
metricsAddress: "0"
leaderElection: true
dns:
  namespace: kube-system
  selector:
    k8s-app: kube-dns
  addresses: []
```

`caFile` is optional for certificates signed by a public certificate authority. TLS verification cannot be disabled. Tokens are read for each request; the trusted CA certificate is loaded at startup.

The following environment variables override their corresponding YAML fields:

| Variable | Field |
|---|---|
| `GFW_API_URL` | `apiURL` |
| `GFW_TOKEN_FILE` | `tokenFile` |
| `GFW_CA_FILE` | `caFile` |
| `GFW_NAMESPACE` | `namespace` |
| `GFW_REFRESH_INTERVAL` | `refreshInterval` |
| `GFW_HTTP_TIMEOUT` | `httpTimeout` |
| `GFW_IPV6_DISABLED` | `ipv6Disabled` |

No variable contains the token itself. Application configuration is immutable during the process lifetime; changing settings requires a restart. Token files and mock API fixtures are the explicitly reloadable exceptions.

## DNS resolvers

By default, a single network peer combines the `kube-system` namespace **and** the `k8s-app=kube-dns` label. It allows only TCP/UDP port 53.

For NodeLocal DNS, specify the resolver's exact IP addresses:

```yaml
dns:
  addresses:
    - 169.254.20.10
```

A nonempty address list replaces DNS pod selection. Only individual IPv4 addresses are accepted, without CIDRs or IPv6. NodeLocal DNS behavior depends on its network mode and the CNI; verify these flows on the target platform.

## Mock API

```yaml
address: ":8443"
tokenFile: /secrets/token/tokens
profilesFile: /profiles/profiles.yaml
certificateFile: /secrets/tls/tls.crt
keyFile: /secrets/tls/tls.key
```

The server listens on IPv4. Secrets and ConfigMaps are mounted as whole directories, without `subPath`, to receive updates projected by Kubernetes.
