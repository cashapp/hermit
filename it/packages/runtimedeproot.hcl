description = "runtime dependency root pkg"
binaries = ["runtimedeproot"]

source = "${env}/bin/runtimedeproot.tar.gz"

version "1.0.0" {
  runtime-dependencies=[
    "runtimedep1-2.0.0",
    "runtimedep2-1.0.0"
  ]
}
