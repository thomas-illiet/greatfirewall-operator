# Development

## Conventions

- Use English for documentation, comments, logs, errors, diagrams, and examples.
- Give each package one responsibility and keep entry points minimal.
- Prefer short functions, early returns, and names that describe the operation.
- The quality check limits production functions to 60 lines and production files to 250 lines.
- Format structures and test tables across multiple lines; separate standard library imports from dependencies.
- Add GoDoc comments to every named function and method, exported or private, including tests and interfaces.
- Add inline comments for security decisions: persistent revocation, DNS, ownership, and cache retention.
- Explain contracts and reasoning in comments without paraphrasing every statement.
- Use `slog` for logs, Cobra for commands, and Viper for configuration.

## Commands

```sh
make fmt
make verify
make build
```

Coverage includes statements throughout the module, including executables and internal tools. The threshold uses exact counters: 89.95% does not pass. No application file is excluded from the denominator. Reports include packages with low coverage to make limitations visible.

## Kubernetes integration

Tests with a fake client verify individual state transitions. An `envtest` integration test exercises the manager, informers, leader election, and reconciliation against a real API server.

```sh
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.23
export KUBEBUILDER_ASSETS="$(setup-envtest use -p path 1.35.x)"
make verify
```

Without `KUBEBUILDER_ASSETS`, the integration test is explicitly skipped; unit tests can still run. CI configures this environment so integration tests are not skipped.

## Network tests

```sh
go install sigs.k8s.io/kind@v0.31.0
make e2e
```

The script requires Docker, Kind, kubectl, Helm, jq, and OpenSSL. It creates its own kubeconfig and deletes only its temporary cluster. Test workloads explicitly disable IPv6 using sysctls allowed in that test cluster; the operator does not inject these settings into production workloads.

Two servers each listen on two ports. Tests allow different destination/port pairs and verify that the crossed pairs are blocked: testing a closed port alone would not prove filtering.

The scenario also verifies DNS, 404 responses, API shutdown, operator restart, and recovery after a new valid response.
