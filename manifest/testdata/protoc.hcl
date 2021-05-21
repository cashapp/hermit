description = "protoc is a compiler for protocol buffers definitions files."
binaries = ["bin/protoc"]
test = "protoc --version"

darwin {
  source = "https://github.com/protocolbuffers/protobuf/releases/download/v${version}/protoc-${version}-osx-x86_64.zip"
}

linux {
  source = "https://github.com/protocolbuffers/protobuf/releases/download/v${version}/protoc-${version}-linux-x86_64.zip"
}

version "3.7.1" {}
version "3.14.0" {}
version "3.15.0" {}
version "3.15.8" {}

channel "stable" {
  update = "24h"
  version = "3.*"
}