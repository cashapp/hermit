description = "Test package three"
env = {FOO: "bar", TESTBIN3VERSION: "${version}"}
source = "${env}/packages/testbin3.foo"
on unpack {
    copy { from = "testbin3/testbin3" to = "${root}/testbin3" mode = 0755 }
}
dont-extract = true
binaries = ["testbin3"]
version "3.0.0" "3.0.1" {}
