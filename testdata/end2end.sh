#!/usr/bin/env bash

set -xeo pipefail

cd  "$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

trap "rm -rf $PWD/env" EXIT INT

rm -rf ./env
mkdir ./env
cd ./env

export HERMIT_REGISTRY=/opt/spack/build_cache

hermit clean -a
hermit init .
hermit install go protobuf openjdk
hermit env
hermit env set GO111MODULE off
ls ./bin

cp ../files/* .

./bin/go run main.go

mkdir out
./bin/protoc test.proto --cpp_out=out
test -r out/test.pb.cc
test -r out/test.pb.h
./bin/javac hello.java
./bin/java hello

hermit env
