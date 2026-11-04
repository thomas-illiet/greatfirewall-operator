# Operations

## Logs and events

Processes use `log/slog` with JSON output. Logs from `controller-runtime` use the same handler through the logr adapter. `--log-level` controls verbosity.

```sh
kubectl -n greatfirewall-system logs deployment/gfw-greatfirewall
kubectl get events -A --field-selector reason=ProfileSyncFailed
```

Useful events include `InvalidProfile`, `UnsupportedNetwork`, `ProfileSyncFailed`, and `ConcurrentPolicy`. Profile names identify affected objects; secrets and API error response bodies are not logged.

## Metrics

`metrics.enabled: true` enables an HTTPS endpoint on port 8444 with Kubernetes authentication and authorization through SubjectAccessReview. It is disabled by default. The chart grants the operator the required delegation permissions; separately grant the collector's account `get` access to the non-resource URL `/metrics`.

- `greatfirewall_reconciliations_total{outcome}`: successful reconciliations and Kubernetes errors.
- `greatfirewall_profile_sync_error{namespace,profile}`: use of previous state after a failed refresh.
- Standard `controller-runtime` metrics: work queues, reconciliation durations, and errors.

The operator's HTTP probes report process availability. They do not prove that the external API is available or that the CNI has applied every rule.

## Token rotation

1. Generate a new cryptographically random token.
2. Add it to the API's accepted token file while retaining the old token.
3. Wait for the API's Secret projection to update, then replace the operator's token file.
4. Verify that synchronization succeeds with the new token.
5. Remove the old token from the API's accepted token file.

The mock API accepts one token per line, up to eight tokens. The client requires exactly one token. Kubernetes projects Secret changes asynchronously, so an overlap period is essential. Do not use a `subPath` mount.

Rotating the server certificate or trusted certificate authority requires a rolling restart of the affected processes.

## Troubleshooting

| Symptom | Check |
|---|---|
| Pod traffic is not filtered | Pod label, CNI compatibility, and other policies selecting the pod |
| Unknown profile | Authenticated 404 response and revoked snapshot |
| `401` / `403` | Secret, Secret key, mounted path, and rotation overlap |
| TLS rejected | CA, certificate SAN, expiration, and configured URL |
| DNS unavailable | Valid profile, DNS selectors, NodeLocal addresses, and CNI behavior |
| Reconciliation error | RBAC, name collisions, or Kubernetes unavailability |
| Old rules retained | External synchronization failure; expected behavior without expiration |

A resource with an expected name but incompatible ownership markers is never adopted automatically. Inspect and resolve the collision explicitly.
