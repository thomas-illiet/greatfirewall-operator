#!/usr/bin/env bash
# Verify real packet filtering on an isolated, disposable IPv4 Kind cluster.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
cluster="gfw-e2e-${RANDOM}"
temporary=$(mktemp -d)
export KUBECONFIG="$temporary/kubeconfig"
created=false
mkdir -p reports

# cleanup preserves diagnostics, then removes only the cluster created by this run.
cleanup() {
  result=$?
  if "$created"; then
    kubectl -n greatfirewall-system logs deployment/gfw-greatfirewall \
      > reports/e2e-operator.log 2>&1 || true
    kubectl get networkpolicies -A -o yaml > reports/e2e-policies.yaml 2>/dev/null || true
    if [[ "$result" != 0 ]]; then
      kubectl get pods -A -o wide || true
      kubectl get events -A --sort-by=.metadata.creationTimestamp | tail -40 || true
    fi
    kind delete cluster --name "$cluster"
  fi
  rm -rf "$temporary"
  exit "$result"
}
trap cleanup EXIT

# wait_policy waits for a profile policy to reach the requested authorization state.
wait_policy() {
  local wanted=$1
  for ((attempt=0; attempt<120; attempt++)); do
    if kubectl -n gfw-test get networkpolicy -o json | jq -e --arg wanted "$wanted" '
      .items[] | select(.metadata.annotations["greatfirewall/profile-name"] == "dev-sandbox") |
      .metadata.annotations["greatfirewall/state"] == $wanted
    ' >/dev/null; then
      return 0
    fi
    sleep 2
  done
  echo "Policy did not reach state: $wanted" >&2
  return 1
}

# allowed retries through CNI convergence and requires an actual successful HTTP response.
allowed() {
  local address=$1
  for ((attempt=0; attempt<20; attempt++)); do
    if kubectl -n gfw-test exec client -- wget -q -T 2 -O - "$address" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "Expected access to $address" >&2
  return 1
}

# denied proves a fresh connection cannot reach an independently running destination.
denied() {
  if kubectl -n gfw-test exec client -- wget -q -T 2 -O - "$1" >/dev/null 2>&1; then
    echo "Unexpected access to $1" >&2
    return 1
  fi
}

for executable in kind docker kubectl helm jq openssl; do
  command -v "$executable" >/dev/null
done
if [[ ${SKIP_BUILD:-0} != 1 ]]; then
  make images
fi

kind create cluster --name "$cluster" --config test/e2e/kind.yaml --kubeconfig "$KUBECONFIG"
created=true
kind load docker-image greatfirewall/operator:0.1.0 greatfirewall/mock-api:0.1.0 --name "$cluster"
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.31.0/manifests/calico.yaml
kubectl -n kube-system rollout status daemonset/calico-node --timeout=240s
kubectl wait --for=condition=Ready nodes --all --timeout=180s

kubectl apply -f test/e2e/targets.yaml
kubectl -n gfw-test rollout status deployment/targets --timeout=180s
addresses=$(kubectl -n gfw-test get pods -l app=target -o json)
first=$(jq -r '.items[0].status.podIP' <<< "$addresses")
second=$(jq -r '.items[1].status.podIP' <<< "$addresses")

bash scripts/demo-secrets.sh greatfirewall-system gfw
cat > "$temporary/profile-values.yaml" <<EOF
api:
  refreshInterval: 3s
mock:
  profiles:
    dev-sandbox:
      profile: dev-sandbox
      revision: "e2e-1"
      egress:
        - cidr: $first/32
          ports:
            - protocol: TCP
              port: 8080
        - cidr: $second/32
          ports:
            - protocol: TCP
              port: 8081
EOF
helm install gfw charts/greatfirewall -n greatfirewall-system \
  -f examples/values-demo.yaml -f "$temporary/profile-values.yaml" --wait --timeout 180s
kubectl -n gfw-test wait --for=create networkpolicy/greatfirewall-deny-egress --timeout=60s
kubectl apply -f test/e2e/client.yaml
kubectl -n gfw-test wait --for=condition=Ready pod/client --timeout=120s
wait_policy allowed

allowed "http://$first:8080"
allowed "http://$second:8081"
denied "http://$first:8081"
denied "http://$second:8080"
kubectl -n gfw-test exec client -- timeout 10 nslookup kubernetes.default.svc.cluster.local >/dev/null
kubectl -n gfw-test exec client -- sh -c \
  'test "$(cat /proc/sys/net/ipv6/conf/all/disable_ipv6)" = 1 && test -z "$(cat /proc/net/if_inet6)"'

# A missing profile is a durable tombstone, not a temporary API outage.
kubectl -n greatfirewall-system get configmap gfw-greatfirewall-profiles -o json > "$temporary/profiles.json"
kubectl -n greatfirewall-system patch configmap gfw-greatfirewall-profiles --type merge \
  -p '{"data":{"profiles.yaml":"profiles: {}\n"}}'
wait_policy denied
sleep 3
denied "http://$first:8080"
if kubectl -n gfw-test exec client -- timeout 5 nslookup kubernetes.default.svc.cluster.local >/dev/null 2>&1; then
  echo "DNS remained accessible after revocation" >&2
  exit 1
fi

kubectl -n greatfirewall-system scale deployment/gfw-greatfirewall-mock --replicas=0
kubectl -n greatfirewall-system rollout restart deployment/gfw-greatfirewall
kubectl -n greatfirewall-system rollout status deployment/gfw-greatfirewall --timeout=120s
wait_policy denied
denied "http://$first:8080"

# A new valid response is required before permissions can return.
jq '{data: .data}' "$temporary/profiles.json" > "$temporary/restore.json"
kubectl -n greatfirewall-system patch configmap gfw-greatfirewall-profiles \
  --type merge --patch-file "$temporary/restore.json"
kubectl -n greatfirewall-system scale deployment/gfw-greatfirewall-mock --replicas=1
kubectl -n greatfirewall-system rollout status deployment/gfw-greatfirewall-mock --timeout=120s
wait_policy allowed
allowed "http://$first:8080"
echo "PASS: IPv4 grants, denied cross-pairs, DNS, IPv6 disabled, revocation, outage, restart and recovery."
