// Package manifest sources.
sources = ["file:///#PWD/../packages/.git"]
env = {
  GOBIN: "${HERMIT_ENV}/out/bin",
  PATH: "${GOBIN}:${PATH}",
}