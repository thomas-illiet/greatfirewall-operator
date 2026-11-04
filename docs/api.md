# Profiles API

The machine-readable contract is available in [openapi.yaml](openapi.yaml).

## Request

`GET /v1/profiles/{name}` requires HTTPS and the `Authorization: Bearer <token>` header. The base URL may include a path prefix. Tokens are never accepted as URL parameters.

The name must be a nonempty Kubernetes label value: at most 63 characters, using letters, digits, hyphens, periods, and underscores according to Kubernetes validation rules.

## Valid response

```json
{
  "profile": "dev-sandbox",
  "revision": "1",
  "egress": [
    {
      "cidr": "192.0.2.10/32",
      "ports": [{"protocol": "TCP", "port": 443}]
    }
  ]
}
```

The profile name must match the request. The revision must be a nonempty string of at most 256 characters. `egress` is required and may be an empty list. An IP address without a prefix becomes a `/32`; IPv4 CIDRs are normalized. Supported protocols are `TCP` and `UDP`, with ports from 1 to 65535.

Responses are limited to 512 KiB, 1,024 rules, and 128 ports per rule. Unknown fields, trailing JSON data, IPv6 addresses, and missing ports invalidate the entire response. Profiles are never truncated to fit these limits.

## States

| Response | Effect |
|---|---|
| Valid `200` | Replace permissions and add cluster DNS |
| `200` with an empty list | Allow cluster DNS only |
| Authenticated `404` | Persist revocation; block all egress, including DNS |
| `401` / `403` | Retain the last known state and report the authentication error |
| Other error, timeout, or invalid response | Retain the last known state |
| No usable previous state | Remain in deny-all mode |

Revoked state is also retained during outages. Redirects are never followed. Error response bodies are never copied into logs.

## Fixtures

```yaml
profiles:
  dev-sandbox:
    profile: dev-sandbox
    revision: "1"
    egress:
      - cidr: 192.0.2.10
        ports:
          - protocol: TCP
            port: 443
```

The mock server rereads and validates the entire file for each authenticated request. `profiles: {}` makes all profiles absent. A malformed file produces a server error, never an accidental revocation of every profile.

Authentication happens before lookup: an invalid token returns `401` even when the requested profile does not exist. Only `/healthz` and `/readyz` are accessible without a token; they disclose no configuration.
