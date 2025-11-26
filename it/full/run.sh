#!/usr/bin/env bash

set -eo pipefail

. ../common.sh

beforeAll() {
  fakeRelease release/canary

  export PATH=$PWD/userbin:$PWD:$PATH
  export HERMIT_STATE_DIR=$PWD/state
  export HERMIT_BIN_INSTALL_DIR=$PWD/userbin

  pushd ../packages
  git init --template=/dev/null .
  git add .
  git config user.email "you@example.com"
  git config user.name "Your Name"
  git config commit.gpgsign false
  git commit -m 'test commit'
  popd
}

afterAll() {
  afterEach
  rm -rf ./release
  rm -rf ../packages/.git
  rm hermit
}

setupEnv() {
  from=$1
  to=$2
  cp -R "${from}" "${to}"

  ESCAPED_PWD=$(printf '%s\n' "$PWD" | sed -e 's/[\/&]/\\&/g')
  sed -i.bak "s/#PWD/${ESCAPED_PWD}/g" "${to}/bin/hermit.hcl"

  (
    cd ../testbins
    tar cvzf testbin1.tar.gz testbin1
    tar cvzf testbin2.tar.gz testbin2
    tar cvzf testbin3.tar.gz testbin3

    cd fake
    tar cvzf ../faketestbin2.tar.gz testbin2
  )
  mv ../testbins/*.tar.gz "${to}/bin/"
  tar -xf ../gitsource.tgz -C "${to}"
}

beforeEach() {
  setupEnv env testenv
  setupEnv oldenv testoldenv
  setupEnv env anotherenv
  setupEnv env isolatedenv1
  setupEnv env isolatedenv2
  mkdir -p ./userbin
}

afterEach() {
  rm -rf ./testenv
  rm -rf ./testoldenv
  rm -rf ./anotherenv
  rm -rf ./isolatedenv1
  rm -rf ./isolatedenv2
  rm -rf ./userbin

  # some downloaded packages might not have write permission
  chmod -f -R u+w ./state 2> /dev/null || true
  rm -rf ./state
}

runTests() {
  beforeEach
  ~/.local/bin/shellspec -s $1
  afterEach
}

trap afterAll EXIT

beforeAll
runTests /bin/zsh
runTests /bin/bash
