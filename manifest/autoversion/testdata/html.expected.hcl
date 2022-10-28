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

sha256sums = {
  "https://golang.org/dl/go1.17.5.linux-amd64.tar.gz": "bd78114b0d441b029c8fe0341f4910370925a4d270a6a590668840675b0c653e",
  "https://golang.org/dl/go1.17.5.darwin-amd64.tar.gz": "2db6a5d25815b56072465a2cacc8ed426c18f1d5fc26c1fc8c4f5a7188658264",
  "https://golang.org/dl/go1.17.5.darwin-arm64.tar.gz": "111f71166de0cb8089bb3e8f9f5b02d76e1bf1309256824d4062a47b0e5f98e0",
}
