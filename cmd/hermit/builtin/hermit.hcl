description = "Hermit manages the installation of separate isolated per-project sets of tools for Linux and Mac."
test = "hermit --version"
binaries = ["hermit"]

on unpack {
  rename { from = "${root}/hermit-${os}-${arch}" to = "${root}/hermit" }
}

channel "stable" {
  source = "https://github.com/cashapp/hermit/releases/download/stable/hermit-${os}-${arch}.gz"
  update = "24h"
}
