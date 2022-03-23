description = "runtime dependency pkg 1"
binaries = ["runtimedep1"]

source = "${env}/bin/runtimedep1.tar.gz"

version "1.0.0" {
  env = {
    RUNTIME_DEP_1_VERSION: "1.0.0"
  }
  runtime-dependencies = [ "runtimedep2-1.0.0" ]
}

version "2.0.0" {
  env = {
    RUNTIME_DEP_1_VERSION: "2.0.0"
  }
  runtime-dependencies = [ "runtimedep2-2.0.0" ]
}
