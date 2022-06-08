#!/bin/bash

# This is a helper file for integration tests, and should not be used directly.
# Instead, this should be sourced from the IT specific sub-folders

if [ ! -z "$HERMIT_ENV" ]; then
  echo "Error: Deactivate Hermit environment before running integration tests"
  exit 1
fi

if [ ! -f ~/.local/bin/shellspec ]; then
  echo "Installing ShellSpec"
  curl -fsSL https://git.io/shellspec | sh -s -- --yes
fi

if [ ! -z $"HERMIT_EXE" ]; then
  unset HERMIT_EXE
fi

# Creates a "fake" release directory and sets HERMIT_DIST_URL to refer to it.
# This function expects the basename of the first argument to be the
# channel name, e.g., "release/canary" or "release/stable"
fakeRelease() {
  DIR=$1
  CHANNEL="$(basename "${DIR}")"

  echo "Compiling hermit"
  (
    . ../../bin/activate-hermit
    go build -ldflags "-X main.channel=${CHANNEL}" -o hermit ../../cmd/hermit
  )

  OS=$(../../bin/go version | awk '{print $NF}' | cut -d/ -f1)
  ARCH=$(../../bin/go version | awk '{print $NF}' | cut -d/ -f2)
  mkdir -p "$DIR"
  gzip -c hermit > "$DIR/hermit-${OS}-${ARCH}.gz"
  ./hermit gen-installer --dest="${DIR}/install.sh"

  export HERMIT_DIST_URL=file://$PWD/$DIR
  echo $HERMIT_DIST_URL
}
