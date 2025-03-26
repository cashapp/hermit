description = "Test package four"
source = "${env}/packages/testbin4"
binaries = ["testbin4"]
version "4.0.0"  {}
requires = ["testbin1"]

env = {
  "TESTBIN4_ROOT": "${HERMIT_BIN}"
}
