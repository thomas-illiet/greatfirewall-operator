# Validation report

Local validation was performed on **November 4, 2026**, on macOS ARM64 with Go 1.26.3 and Docker Desktop. No production deployment or remote image publication was performed.

## Go quality

`make verify build` passed with the Kubernetes 1.35.0 `envtest` integration API server enabled.

| Check | Result |
|---|---|
| `gofmt` formatting | Passed |
| `go vet` analysis | Passed |
| Function, method, and interface documentation | Passed, including tests |
| Production code size | Functions ≤ 60 lines; files ≤ 250 lines |
| Tests with `-race` | Passed |
| Exact global coverage | **626 / 649 statements — 96.4561%** |
| Coverage threshold ≥ 90% | Passed without rounding |
| Both executable builds | Passed |

Coverage includes every Go package in the module, including commands and internal tools. The small `main` entry points remain in the denominator; the threshold applies to the whole project, not to each package individually.

Evidence: [verification log](../reports/verification.log), [per-package counters](../reports/coverage.txt), [HTML report](../reports/coverage.html), and [raw Go counters](../reports/coverage.out).

## Authentication and state transitions

Tests cover valid, missing, and incorrect tokens, rotation, rejection of untrusted certificates, rejection of redirects, and the absence of secrets in returned errors. Response validation rejects malformed JSON and IPv6 profiles, including IPv4 addresses represented as IPv6.

Controller tests cover policy sharing, repair, label removal, baseline retention, and refusal to modify third-party policies. Cache tests cover invalid responses, outages, write conflicts, restoration, and persistent revocation. A failed cache write does not prevent discovering a subsequent revocation.

## Images and Helm

The `greatfirewall/operator:0.1.0` and `greatfirewall/mock-api:0.1.0` images were built locally. `helm lint` and rendering of the demo chart passed. Helm's only informational message recommends adding a chart icon.

Evidence: [image build log](../reports/images.log).

## Real network tests

The `scripts/e2e.sh` scenario passed on **Kind 0.31.0, Kubernetes 1.35.0, and Calico 3.31.0**. It uses local images, an authenticated HTTPS API, and an independent temporary kubeconfig.

- Two IPv4 destinations each listen on two ports; only authorized pairs are reachable.
- Crossed destination/port pairs remain blocked.
- Cluster DNS is reachable for a valid profile and blocked after revocation.
- IPv6 is disabled in the workload's network namespace, with no IPv6 address present.
- A valid profile followed by a 404 loses its permissions.
- Stopping the API and restarting the operator does not restore revoked permissions.
- A new valid response restores the intended access.

The test cluster is deleted when the scenario ends. Retained evidence includes the [scenario log](../reports/e2e.log), [operator logs](../reports/e2e-operator.log), and [final policies](../reports/e2e-policies.yaml).

## Scope of results

Network verification covers this IPv4 platform with Calico. It does not replace validation on the production CNI, particularly for NodeLocal DNS. Established connections, additive policies, and `hostNetwork` traffic remain subject to the [documented limitations](security.md).

The included CI workflow reproduces the Go and network checks, but no remote CI execution result is claimed.
