# Installation

## Prepare the cluster

The cluster must use IPv4, with IPv6 disabled inside workload network namespaces and a networking component that enforces NetworkPolicies. `platform.ipv6Disabled: true` acknowledges this platform configuration; it does not disable IPv6 itself.

Reserve an administrative namespace for the operator. Workload users must not be able to modify its Secrets or cache ConfigMaps, or broaden the policies selecting protected pods.

The chart declares Kubernetes 1.32 or later. The integration API server and Kind tests target Kubernetes 1.35. Verify enforcement on your Kubernetes version and CNI before production use.

## Build the images

```sh
make images
```

The images are `greatfirewall/operator:0.1.0` and `greatfirewall/mock-api:0.1.0`. To use a private registry, retag and publish them, then update `image.repository` and `mock.image.repository` in the Helm values. The scripts do not publish images to a remote registry.

## External API

Create a Secret with a `token` key populated from a private file. For a private certificate authority, also create a Secret containing `ca.crt`.

```yaml
platform:
  ipv6Disabled: true
api:
  url: https://profiles.example.internal
  tokenSecret: gfw-api-auth
  caSecret: gfw-api-ca
mock:
  enabled: false
```

```sh
helm upgrade --install gfw charts/greatfirewall \
  --namespace greatfirewall-system --create-namespace \
  --values values-production.yaml --wait
```

## Demo

`scripts/demo-secrets.sh` creates a random token and a self-signed certificate valid for two days. The certificate's DNS name depends on the release name and namespace passed to the script. Temporary files are private and removed on exit.

```sh
bash scripts/demo-secrets.sh greatfirewall-system gfw
helm upgrade --install gfw charts/greatfirewall \
  --namespace greatfirewall-system \
  --values examples/values-demo.yaml --wait
```

The script refuses to overwrite existing Secrets. For a lasting installation, use your usual certificate and secret management process.

## Prepare workloads

The operator installs a baseline policy in every observed namespace. It selects only pods labelled `greatfirewall/profile`. Prepare namespaces before starting untrusted workloads, then verify that the CNI actually enforces the rules.

For a Deployment, place the label under `spec.template.metadata.labels`. A label on the Deployment itself or on the namespace does not select any pods.

## Upgrade and uninstall

Helm upgrades restart the operator when its configuration changes. NetworkPolicies and cache ConfigMaps created at runtime are not Helm release resources; they remain after `helm uninstall` to avoid implicitly opening network access.

Removing them requires an explicit administrative action after the affected workloads have been stopped or migrated. Deleting a namespace deletes its policies; the global cache can be retained to preserve the last known profiles.
