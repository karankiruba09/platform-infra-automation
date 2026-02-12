#!/usr/bin/env bash
set -euo pipefail

# Required:
#   HARBOR_REGISTRY   e.g. harbor.example.internal
#   HARBOR_PROJECT    e.g. library
# Optional:
#   CONTOUR_URL       defaults to release-1.31 manifest
#   OUT_FILE          defaults to contour-harbor.yaml

: "${HARBOR_REGISTRY:?HARBOR_REGISTRY is required}"
: "${HARBOR_PROJECT:?HARBOR_PROJECT is required}"

CONTOUR_URL="${CONTOUR_URL:-https://raw.githubusercontent.com/projectcontour/contour/release-1.31/examples/render/contour.yaml}"
OUT_FILE="${OUT_FILE:-contour-harbor.yaml}"
TMP_FILE="contour-upstream.yaml"

curl -fsSLo "${TMP_FILE}" "${CONTOUR_URL}"

echo "Downloaded ${CONTOUR_URL}"

SRC_CONTOUR_IMAGE="$(grep -Eo 'ghcr.io/projectcontour/contour:[^"[:space:]]+' "${TMP_FILE}" | head -n1)"
SRC_ENVOY_IMAGE="$(grep -Eo 'docker.io/envoyproxy/envoy:[^"[:space:]]+' "${TMP_FILE}" | head -n1)"

if [[ -z "${SRC_CONTOUR_IMAGE}" || -z "${SRC_ENVOY_IMAGE}" ]]; then
  echo "Unable to detect upstream contour/envoy image references in ${TMP_FILE}" >&2
  exit 1
fi

CONTOUR_TAG="${SRC_CONTOUR_IMAGE##*:}"
ENVOY_TAG="${SRC_ENVOY_IMAGE##*:}"

DST_CONTOUR_IMAGE="${HARBOR_REGISTRY}/${HARBOR_PROJECT}/contour:${CONTOUR_TAG}"
DST_ENVOY_IMAGE="${HARBOR_REGISTRY}/${HARBOR_PROJECT}/envoy:${ENVOY_TAG}"

printf 'Source contour image: %s\n' "${SRC_CONTOUR_IMAGE}"
printf 'Source envoy image:   %s\n' "${SRC_ENVOY_IMAGE}"
printf 'Target contour image: %s\n' "${DST_CONTOUR_IMAGE}"
printf 'Target envoy image:   %s\n' "${DST_ENVOY_IMAGE}"

docker pull "${SRC_CONTOUR_IMAGE}"
docker pull "${SRC_ENVOY_IMAGE}"

docker tag "${SRC_CONTOUR_IMAGE}" "${DST_CONTOUR_IMAGE}"
docker tag "${SRC_ENVOY_IMAGE}" "${DST_ENVOY_IMAGE}"

docker push "${DST_CONTOUR_IMAGE}"
docker push "${DST_ENVOY_IMAGE}"

cp "${TMP_FILE}" "${OUT_FILE}"
sed -i "s|${SRC_CONTOUR_IMAGE}|${DST_CONTOUR_IMAGE}|g" "${OUT_FILE}"
sed -i "s|${SRC_ENVOY_IMAGE}|${DST_ENVOY_IMAGE}|g" "${OUT_FILE}"

echo "Wrote updated manifest: ${OUT_FILE}"
echo "Verify image replacements:"
grep -n 'image:' "${OUT_FILE}"
