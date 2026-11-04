#!/usr/bin/env bash
# Create disposable demo credentials without printing tokens or private keys.
set -euo pipefail
umask 077

namespace=${1:-greatfirewall-system}
release=${2:-gfw}
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT

openssl rand -base64 48 > "$temporary/token"
hostname="${release}-greatfirewall-mock.${namespace}.svc"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -keyout "$temporary/tls.key" -out "$temporary/tls.crt" \
  -subj "/CN=${hostname}" \
  -addext "subjectAltName=DNS:${hostname},DNS:${hostname}.cluster.local" \
  > /dev/null 2>&1

kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$namespace" create secret generic gfw-demo-auth \
  --from-file=token="$temporary/token" \
  --from-file=tokens="$temporary/token"
kubectl -n "$namespace" create secret generic gfw-demo-tls \
  --from-file=tls.crt="$temporary/tls.crt" \
  --from-file=tls.key="$temporary/tls.key" \
  --from-file=ca.crt="$temporary/tls.crt"

echo "Demo Secrets created. The self-signed certificate expires in two days."
