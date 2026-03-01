description = "A test package."
binaries = ["bin/test"]
source = "https://example.com/test-${version}.tar.gz"

version "1.0.0" "1.2.3" {
  auto-version {
    version-pattern = "v(.*)"

    json {
      url = "https://example.com/api/releases"
      jq = ".releases[].tag_name"
    }
  }
}
