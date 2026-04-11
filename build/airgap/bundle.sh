#!/bin/bash
set -euo pipefail

REGISTRY="${REGISTRY:-registry.local:5000}"
VERSION="${VERSION:-v1.0.0}"
BUNDLE_DIR="${BUNDLE_DIR:-./airgap-bundle}"

IMAGES=(
  "chaosplane-hq/chaosplane-operator:${VERSION}"
  "chaosplane-hq/chaosplane-daemon:${VERSION}"
  "chaosplane-hq/chaosplane-agent:${VERSION}"
  "chaosplane-hq/chaosplane-api:${VERSION}"
  "chaosplane-hq/chaosplane-web:${VERSION}"
  "postgres:16-alpine"
  "redis:7-alpine"
)

mkdir -p "${BUNDLE_DIR}/images"

echo "Pulling and saving images..."
for img in "${IMAGES[@]}"; do
  filename=$(echo "${img}" | tr '/:' '_')
  echo "  ${img} -> ${filename}.tar"
  docker pull "${img}" 2>/dev/null || echo "  (skipping pull, using local)"
  docker save "${img}" -o "${BUNDLE_DIR}/images/${filename}.tar"
done

echo "Copying Helm chart..."
cp -r ../helm-charts/charts/chaosplane "${BUNDLE_DIR}/chart"

cat > "${BUNDLE_DIR}/load-images.sh" << 'LOADER'
#!/bin/bash
set -euo pipefail
REGISTRY="${REGISTRY:-registry.local:5000}"
for tarball in images/*.tar; do
  echo "Loading ${tarball}..."
  docker load -i "${tarball}"
  image=$(docker load -i "${tarball}" 2>&1 | grep "Loaded image" | awk '{print $NF}')
  if [ -n "${image}" ]; then
    new_tag="${REGISTRY}/${image##*/}"
    docker tag "${image}" "${new_tag}"
    docker push "${new_tag}"
    echo "  Pushed ${new_tag}"
  fi
done
echo "All images loaded and pushed to ${REGISTRY}"
LOADER
chmod +x "${BUNDLE_DIR}/load-images.sh"

cat > "${BUNDLE_DIR}/install.sh" << 'INSTALLER'
#!/bin/bash
set -euo pipefail
REGISTRY="${REGISTRY:-registry.local:5000}"
NAMESPACE="${NAMESPACE:-chaosplane}"
RELEASE="${RELEASE:-chaosplane}"

echo "Installing ChaosPlane in air-gap mode..."
helm upgrade --install "${RELEASE}" ./chart \
  --namespace "${NAMESPACE}" --create-namespace \
  --set global.imageRegistry="${REGISTRY}" \
  --set global.airgap=true \
  --set operator.image.repository="${REGISTRY}/chaosplane-operator" \
  --set daemon.image.repository="${REGISTRY}/chaosplane-daemon" \
  --set agent.image.repository="${REGISTRY}/chaosplane-agent" \
  --set api.image.repository="${REGISTRY}/chaosplane-api" \
  --set web.image.repository="${REGISTRY}/chaosplane-web"

echo "ChaosPlane installed successfully in namespace ${NAMESPACE}"
INSTALLER
chmod +x "${BUNDLE_DIR}/install.sh"

echo ""
echo "Air-gap bundle created at ${BUNDLE_DIR}/"
echo "Transfer to air-gapped environment, then run:"
echo "  1. ./load-images.sh    (load images into local registry)"
echo "  2. ./install.sh        (install via Helm)"
