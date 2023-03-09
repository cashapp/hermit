description = "Test package one"
env = {FOO: "bar", TESTBIN1VERSION: "${version}"}
source = "${env}/packages/testbin1.tgz"
binaries = ["testbin1", "dir/testbin2"]

on unpack {
  mkdir { dir = "${root}/dir" }
  symlink { from = "${root}/testbin1" to = "${root}/dir/testbin2" }
}

on exec {
  // Messages can't be sent to the parent process, so we create a directory to indicate the hook worked instead.
  mkdir { dir = "${env}/exec-hook-triggered" }
}

version "1.0.0" "1.0.1" {}
