description = "Runtime dep for testbin1"
env = {FOO: "runtimefoo", BAR: "runtimebar"}
source = "${env}/packages/runtimedep.sh"
binaries = ["runtimedep.sh"]
version "1.0.0" {}
