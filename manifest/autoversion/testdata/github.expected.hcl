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

channel "stable" {
  update = "24h"
  version = "3.*"
}

sha256sums = {
  "https://github.com/jenkins-x/jx/releases/download/v3.2.150/jx-linux-amd64.tar.gz": "5c08292e49d54e238e886e86ec2d158f57fdad5cb716c157a4e3ebf210c0ffac",
  "https://github.com/jenkins-x/jx/releases/download/v3.2.150/jx-darwin-amd64.tar.gz": "050a65ce222be92ecbfc9d56ffbfae4c9b9ebca61bf8744dfa388ca49e57d449",
}
