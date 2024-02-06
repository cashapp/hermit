description = "Zig is a general-purpose programming language and toolchain for maintaining robust, optimal and reusable software"
strip = 1

linux {
  source = "https://ziglang.org/download/${version}/zig-linux-${xarch}-${version}.tar.xz"
}

darwin {
  source = "https://ziglang.org/download/${version}/zig-macos-${xarch}-${version}.tar.xz"
}

version "0.10.0" {
  auto-version {
    html {
      url = "https://ziglang.org/download/"
      css = "h2[id^=release-0]"
    }
  }
}
