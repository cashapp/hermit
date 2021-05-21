description = "Hermit manages the installation of separate isolated per-project sets of tools for Linux and Mac."
test = "hermit --version"
binaries = ["hermit"]

on unpack {
  rename { from = "${root}/hermit-${os}-${arch}" to = "hermit" }
}

channel "canary" {
  source = "https://github.com/cashapp/hermit/releases/download/stable/hermit-${os}.gz"
  update = "24h"
}
