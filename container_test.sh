#!/usr/bin/env bash
# container_test.sh — Build and run the container-isolated onramp tests.
#
# Usage:
#   ./container_test.sh              # build & run all container tests
#   ./container_test.sh -run Stream  # run only tests matching "Stream"
#   ./container_test.sh --build-only # build the image without running
#
# The tests use the embedded go-i2p router exclusively — no external
# I2P router or network access is required.
set -euo pipefail

IMAGE="onramp-container-test"
DOCKERFILE="Dockerfile.test"
RUN_FILTER=""
BUILD_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --build-only) BUILD_ONLY=true ;;
        -run)         shift; RUN_FILTER="$1" ;;
    esac
    shift 2>/dev/null || true
done

echo "==> Building test image ${IMAGE} …"
docker build -f "${DOCKERFILE}" -t "${IMAGE}" .

if "${BUILD_ONLY}"; then
    echo "==> Image built.  (--build-only specified, skipping run)"
    exit 0
fi

# Override ENTRYPOINT args when a -run filter is given.
if [[ -n "${RUN_FILTER}" ]]; then
    echo "==> Running container tests (filter: ${RUN_FILTER}) …"
    docker run --rm "${IMAGE}" \
        go test -v -tags container -run "${RUN_FILTER}" -count=1 -timeout 20m ./...
else
    echo "==> Running container tests …"
    docker run --rm "${IMAGE}"
fi

echo "==> Container tests finished."
