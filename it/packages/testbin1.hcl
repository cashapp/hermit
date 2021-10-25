description = "test package"
binaries = ["testbin1"]

source = "${env}/bin/testbin1.tar.gz"

runtime-dependencies=["testbin2-1.0.0"]

version "1.0.0" {}
