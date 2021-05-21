#!/bin/bash

set -eo pipefail

. ../common.sh

echo "Listing previous releases ..."
PREVIOUS=$(aws s3 ls s3://cash-hermit-packages/release/ | sed -n "s/^.*PRE\ \(.*\)\/.*$/\1/p")

fetchPrevious() {
  echo "Fetching previous versions ..."
  echo "$PREVIOUS" | while read line ; do
    aws s3 cp --recursive s3://cash-hermit-packages/release/$line/files ./files/$line/
  done
}

beforeAll() {
  fakeRelease release/canary

  export HERMIT_STATE_DIR=$PWD/state
  mkdir -p $HERMIT_STATE_DIR/pkg/hermit@canary/
  cp ./hermit $HERMIT_STATE_DIR/pkg/hermit@canary/hermit
}

afterAll() {
  afterEach
  rm hermit
  rm -rf ./files
  rm -rf ./release
  
  chmod -f -R u+w ./state 2> /dev/null || true
  rm -rf ./state
}

beforeEach() {
  VERSION=$1

  cp -R env testenv
  cp ./files/$VERSION/* ./testenv/bin/
  chmod u+x ./testenv/bin/hermit
}

afterEach() {
  rm -rf ./testenv
}

trap afterAll EXIT

runTest() {
  TEST_SHELL=$1
  VERSION=$2
  beforeEach $VERSION
   ~/.local/bin/shellspec -s $TEST_SHELL
  afterEach
}

runTests() {
  VERSION=$1
  echo "Testing with scripts from $VERSION"
  runTest /bin/zsh $VERSION
  runTest /bin/bash $VERSION
}

fetchPrevious
beforeAll
echo "$PREVIOUS" | while read line ; do
   runTests $line
done
