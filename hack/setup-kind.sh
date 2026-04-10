#!/usr/bin/env bash
# setup-kind.sh — Create a kind cluster for chaosplane local development.
set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER_NAME:-chaosplane-dev}"

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "Kind cluster '${CLUSTER_NAME}' already exists. Skipping creation."
  exit 0
fi

echo "Creating kind cluster '${CLUSTER_NAME}'..."

cat <<EOF | kind create cluster --name "${CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30080
        hostPort: 30080
        protocol: TCP
      - containerPort: 30443
        hostPort: 30443
        protocol: TCP
  - role: worker
  - role: worker
EOF

echo "Kind cluster '${CLUSTER_NAME}' created successfully."
kubectl cluster-info --context "kind-${CLUSTER_NAME}"
