description = "Test package one"
env = {
  FOO: "bar",
  TESTBIN1VERSION: "${version}",
  TESTBIN1_ROOT: "${root}",
}
source = "${env}/packages/testbin1.tgz"
binaries = ["testbin1"]
version "1.0.0" "1.0.1" {}
