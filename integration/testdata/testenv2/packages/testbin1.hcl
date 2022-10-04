description = "Test package one"
env = {FOO: "bar", TESTBIN1VERSION: "${version}"}
source = "${env}/packages/testbin1.tgz"
binaries = ["testbin1"]
version "1.0.0" "1.0.1" {}
