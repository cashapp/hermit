#!/bin/bash
#
# Build reproducible hermit binaries for {linux, darwin} on {amd64, arm64}
#
# Environment variables:
# VERSION: if set, version of the hermit binary. Default is a git tag.
# CHANNEL: if set, the hermit channel. Default is "canary".
set -euxo pipefail

VERSION="${VERSION:-$(git describe --tags --dirty --always)}"
CHANNEL="${CHANNEL:-canary}"

SCRIPT_DIR=$(cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
readonly SCRIPT_DIR

OUT_DIR="${SCRIPT_DIR}/r10e-build"
readonly OUT_DIR

REVISION=$(git --work-tree="${SCRIPT_DIR}" --git-dir="${SCRIPT_DIR}/.git" \
  rev-parse HEAD)
readonly REVISION

BUILDER_TAG_NAME="hermit-builder:${REVISION}"
readonly BUILDER_TAG_NAME

#########################################
# Self test
#########################################
err() {
  echo -e "$*" >&2
  exit 1
}

self_test() {
  # Check sha256sum
  command -v sha256sum &>/dev/null || \
    err "sha256sum not found. Please install coreutils first"

  # Check docker
  docker stats --no-stream &>/dev/null || \
    err "Make sure docker daemon is running"
}


self_test
echo "Build reproducible (r10e) hermit executables"
cd "${SCRIPT_DIR}"
docker build -f "${SCRIPT_DIR}/Dockerfile" -t "${BUILDER_TAG_NAME}" \
  --build-arg channel="${CHANNEL}" --build-arg version="${VERSION}" .
docker images "${BUILDER_TAG_NAME}"
rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

docker run --entrypoint=/bin/sh --rm -i -v "${OUT_DIR}":/tmp/ "${BUILDER_TAG_NAME}" << CMD
cp /build/hermit/build/hermits.tar.gz /tmp/
CMD

cd "${OUT_DIR}"
tar xfz hermits.tar.gz
echo "==== HERMIT EXECUTABLE INFO ===="
echo "Reproducible executables created in ${OUT_DIR}."
echo "Executable sha256sum values:"
sha256sum hermit-* | sort -k2
echo
