#!/bin/bash

set -eo pipefail

. ../common.sh

# true if we want to check compatibility with all packages.
# Otherwise, we will only check against a few selected packages
RUN_ALL="false"

while test $# -gt 0; do
  case "$1" in
    -a)
      shift
      RUN_ALL="true"
      ;;
  esac
done

beforeAll() {
  fakeRelease release/canary

  mkdir -p ./testenv
  mkdir -p ./userbin

  export PATH=$PWD/userbin:$PWD:$PATH
  export HERMIT_STATE_DIR=$PWD/state
  export HERMIT_BIN_INSTALL_DIR=$PWD/userbin

  pushd testenv
  hermit init .
}

afterAll() {
  popd
  rm -rf ./testenv
  rm -rf ./userbin
  rm -rf ./release
  chmod -f -R u+w ./state 2> /dev/null || true
  rm -rf ./state
  rm -f hermit
}

trap afterAll EXIT

beforeAll

testSubset() {
  ./bin/hermit test openjdk --level=debug
  ./bin/hermit test hermit --level=debug
}

testAll() {
  ./bin/hermit search -s | xargs ./bin/hermit test --level=debug
}

if [ $RUN_ALL == "true" ]; then
  testAll
else
  testSubset
fi
