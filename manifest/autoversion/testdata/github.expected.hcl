description = "Jenkins X CLI"
test = "jx version"
binaries = ["jx"]

linux {
  source = "https://github.com/jenkins-x/jx/releases/download/v${version}/jx-linux-amd64.tar.gz"
}

darwin {
  source = "https://github.com/jenkins-x/jx/releases/download/v${version}/jx-darwin-amd64.tar.gz"
}

version "3.2.137" "3.2.140" "3.2.150" {
  auto-version {
    github-release = "jenkins-x/jx"
    version-pattern = "v(3\\.\\d+\\.\\d+)"
    ignore-invalid-versions = true
  }
}

version "5.1.233" "5.1.234" {
  auto-version {
    github-release = "jenkins-x/jx"
    version-pattern = "v(5\\.\\d+\\.\\d+)"
    ignore-invalid-versions = true
  }
}

channel "stable" {
  update = "24h"
  version = "3.*"
}
