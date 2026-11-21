#!/usr/bin/env bash
# Creates a kind cluster with the CloudNativePG operator and a test Cluster.
# Usage: dev/kind/setup.sh   (teardown: kind delete cluster --name synccraft)
set -euo pipefail
cd "$(dirname "$0")"

CLUSTER=synccraft
CNPG_CHART_VERSION=0.29.1 # operator 1.30.1

# Docker if it works, else podman (rootless podman needs systemd Delegate=yes,
# see https://kind.sigs.k8s.io/docs/user/rootless/). Override with KIND_EXPERIMENTAL_PROVIDER.
if [[ -z "${KIND_EXPERIMENTAL_PROVIDER:-}" ]]; then
  if docker info >/dev/null 2>&1; then
    # kindest/node is public: don't fail on a configured credential helper that isn't installed.
    helper=$(sed -n 's/.*"credsStore": *"\([^"]*\)".*/\1/p' "${DOCKER_CONFIG:-$HOME/.docker}/config.json" 2>/dev/null || true)
    if [[ -n "$helper" ]] && ! command -v "docker-credential-$helper" >/dev/null; then
      DOCKER_CONFIG=$(mktemp -d)
      export DOCKER_CONFIG
      trap 'rm -rf "$DOCKER_CONFIG"' EXIT
    fi
  else
    export KIND_EXPERIMENTAL_PROVIDER=podman
  fi
fi

if ! kind get clusters | grep -qx "$CLUSTER"; then
  kind create cluster --name "$CLUSTER" --wait 120s
fi
kubectl config use-context "kind-$CLUSTER" >/dev/null

helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null 2>&1 || true
helm repo update cnpg >/dev/null
helm upgrade --install cnpg cnpg/cloudnative-pg --version "$CNPG_CHART_VERSION" \
  --namespace cnpg-system --create-namespace --wait --timeout 5m

kubectl apply -f cluster.yaml
echo "waiting for the CNPG cluster to become ready..."
kubectl -n synccraft wait cluster/pg --for=condition=Ready --timeout=10m
kubectl -n synccraft get cluster,pods,svc
