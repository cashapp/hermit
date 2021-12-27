description = "Go is an open source programming language that makes it easy to build simple, reliable, and efficient software."
binaries = ["bin/*"]
strip = 1
test = "go version"
source = "https://golang.org/dl/go${version}.${os}-${arch}.tar.gz"

version "1.17.3" "1.17.5" {
  auto-version {
    version-pattern = "go([^\\s]+)"

    html {
      url = "https://go.dev/dl/"
      xpath = "//h3/text()"
    }
  }
}
