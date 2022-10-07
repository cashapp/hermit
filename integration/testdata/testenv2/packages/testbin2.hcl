description = "Test package two"
env = {FOO: "bar", TESTBIN2VERSION: "${version}"}
source = "${env}/packages/testbin2"
binaries = ["testbin2"]
version "2.0.0" "2.0.1" {}
