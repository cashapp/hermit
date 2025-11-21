#!/usr/bin/env bash
set -euxo pipefail

SCRIPT_DIR=$(cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd "${SCRIPT_DIR}/.."

for channel in "canary" "stable"; do
  rm -rf build
  # Build on native architecture
  make CHANNEL="${channel}" build
  gunzip -c build/hermit*.gz > build/hermit
  chmod +x build/hermit
  build/hermit version
  PROJECT_DIR=$(mktemp -d)
  build/hermit init "${PROJECT_DIR}"
  BIN_HERMIT_SHA=$(openssl dgst -sha256 "${PROJECT_DIR}/bin/hermit" | awk '{print $NF}')
  BIN_ACTIVATE_HERMIT_SHA=$(openssl dgst -sha256 "${PROJECT_DIR}/bin/activate-hermit" | awk '{print $NF}')
  if ! build/hermit script-sha | grep "${BIN_HERMIT_SHA}" &>/dev/null; then
    echo "(${channel}) Script hermit's sha256 ${BIN_HERMIT_SHA} not found. Please add it to files/script.sha256."
    exit 1
  fi
  if ! build/hermit script-sha | grep "${BIN_ACTIVATE_HERMIT_SHA}" &>/dev/null; then
    echo "(${channel}) Script activate-hermit's sha256 ${BIN_ACTIVATE_HERMIT_SHA} not found. Please add it to files/script.sha256."
    exit 1
  fi
done
echo "File files/script.sha is up-to-date."
# Clean up
rm -rf build