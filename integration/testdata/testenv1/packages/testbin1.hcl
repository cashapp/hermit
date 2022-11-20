description = "Test package one"
env = {FOO: "bar", TESTBIN1VERSION: "${version}"}
source = "${env}/packages/testbin1.tgz"
binaries = ["testbin1"]
on install {
  message { text = "testbin1-${version} hook" }
}
version "1.0.0" "1.0.1" {}
