# Security and guarantees

## Product boundary

The product controls outgoing TCP/UDP connections from selected pods on an IPv4 platform. It does not change ingress policies. Unlabelled pods and workloads sharing the host's network namespace are outside its guarantees.

The platform must disable IPv6, including inside workload network namespaces. Declaring a cluster single-stack does not by itself prove the absence of local IPv6 addresses. The operator rejects all IPv6 permissions and reports protected pods observed with IPv6 addresses or `hostNetwork`. As a precaution, their entire namespace/profile group receives no permissions.

Standard NetworkPolicies do not guarantee filtering for every protocol, including ICMP, or isolation of certain flows to the node. They do not provide a complete security boundary against privileged containers on their own. See the [Kubernetes documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/).

## Operational prerequisites

- A compatible CNI, with effective filtering verified on the nodes.
- A baseline prepared before workloads start; its presence in the API does not prove CNI convergence.
- Permissions preventing workloads from removing their labels or creating permissive policies.
- No overlapping policy that broadens egress for protected pods.
- Platform enforcement of privilege and host networking restrictions.

Kubernetes policy permissions are additive. The operator emits an event when a third-party policy also selects protected pods, but does not delete that policy. Removing the label removes the pod from protection; this version has no admission webhook to prevent that action.

## Tokens and TLS

A random opaque token authenticates the operator to the API. HTTPS authenticates the server and protects tokens and profiles in transit. Untrusted certificates and incorrect server names are rejected.

Tokens are stored in Secrets and mounted read-only. Their values never appear in Helm values, URLs, logs, events, or metrics. The server compares fixed-length digests in constant time and accepts multiple tokens during a controlled rotation.

Kubernetes must protect Secrets at rest and restrict access to the cache. Cache ConfigMaps contain no tokens, but their profile data is security-sensitive and requires integrity protection.

## Outages and revocation

An API outage retains the last permissions without expiration: an obsolete rule may remain active until the service recovers. This availability tradeoff is part of the specified behavior.

A 404 durably replaces the previous state with a revocation, including DNS. Enforcement remains asynchronous, depends on Kubernetes and CNI availability, and does not guarantee immediate termination of established connections.

If persisting a revocation fails, the process retains the revoked state in memory and retries the write. A crash before a durable write is a control-plane limitation; monitor persistence errors and do not treat readiness probes as proof of network enforcement.
