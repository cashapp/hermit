description = "git source test package"
binaries = ["gitbin"]
test = "gitbin"


channel "head" {
  update = "24h"
  source = "${env}/gitsource.git"
}

version "1.0.0" {
  source = "${env}/gitsource.git#v${version}"
}
