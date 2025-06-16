description = "Test package for JSON auto-version"
binaries = ["bin/*"]
test = "test-package version"
source = "https://example.com/download/${version}.tar.gz"

version "1.0.0" "2.0.1" {
  auto-version {
    version-pattern = "v?(.*)"

    json {
      url = "https://api.example.com/releases.json"
      path = "latest.version"
    }
  }
}
