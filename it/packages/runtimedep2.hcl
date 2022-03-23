description = "runtime dependency pkg 2"
binaries = ["runtimedep2"]

source = "${env}/bin/runtimedep2.tar.gz"

version "1.0.0" {
  env = {
    RUNTIME_DEP_2_VERSION: "1.0.0"
  }
}

version "2.0.0" {
  env = {
    RUNTIME_DEP_2_VERSION: "2.0.0"
  }
}
