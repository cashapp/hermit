description = "Terraform version manager"
binaries = ["bin/tfenv"]
source = "https://github.com/tfutils/tfenv/archive/refs/tags/v${version}.tar.gz"
strip = 1

version "3.0.0" {
  auto-version {
    github-release = "terraform-linters/tflint"
  }
}

test = "tfenv --version"
sha256sums = {
  "https://github.com/tfutils/tfenv/archive/refs/tags/v3.0.0.tar.gz": "463132e45a211fa3faf85e62fdfaa9bb746343ff1954ccbad91cae743df3b648",
}
