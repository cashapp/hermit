# The following information is from https://hub.docker.com/r/nixos/nix/tags
FROM nixos/nix:2.7.0@sha256:3a2c7a7e5ca8b7f4c128174e3fe018811640e7a549cd1aed4b1f1a20ed7786a5 as hermit_builder

ARG version=devel
ARG channel=canary

#########################################################
# Step 1: Prepare nixpkgs for deterministic builds
#########################################################
WORKDIR /build
# This commit is tagged as 21.11 in nixpkgs
ENV NIXPKGS_COMMIT_SHA="a7ecde854aee5c4c7cd6177f54a99d2c1ff28a31"

RUN nix-env --option filter-syscalls false -i git && \
    mkdir -p /build/nixpkgs && \
    cd nixpkgs && \
    git init && \
    git remote add origin https://github.com/NixOS/nixpkgs.git && \
    git fetch --depth 1 origin ${NIXPKGS_COMMIT_SHA} && \
    git checkout FETCH_HEAD && \
    cd ..

ENV NIX_PATH=nixpkgs=/build/nixpkgs

#########################################################
# Step 2: Build hermit
#########################################################
RUN mkdir -p /build/hermit

COPY . /build/hermit

RUN cd hermit && \
    nix-shell -p go gnumake \
      --run "make GOOS=linux GOARCH=amd64 VERSION=$version CHANNEL=$channel build && \
             make GOOS=linux GOARCH=arm64 VERSION=$version CHANNEL=$channel build && \
             make GOOS=darwin GOARCH=amd64 VERSION=$version CHANNEL=$channel build && \
             make GOOS=darwin GOARCH=arm64 VERSION=$version CHANNEL=$channel build" && \
    cd build && \
    for f in *.gz; do gunzip "$f"; done && \
    tar cfz hermits.tar.gz hermit-linux-amd64 hermit-linux-arm64 hermit-darwin-amd64 hermit-darwin-arm64

