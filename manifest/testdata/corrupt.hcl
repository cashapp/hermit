description = "protoc is a compiler for protocol buffers definitions files."
binaries = ["bin/protoc"]
test = "protoc --version"

darwin {
  source = "https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/protoc-3.7.1-osx-x86_64.zip"
}

linux {
  surce = "https://github.com/protocolbuffers/protobuf/releases/download/v3.7.1/protoc-3.7.1-linux-x86_64.zip"
}

version "3.7.2" {
    env = {
      PKG_TEST_VAR: "test_value_2"
    }
}

version "3.7.1" {
    env = {
      PKG_TEST_VAR: "=test\"value\""
    }
}

version "4.0.0" {}

channel "stable" {
  update = "24h"
  version = "3.7.*"
}
