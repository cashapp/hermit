description = "Hermit manages the installation of separate isolated per-project sets of tools for Linux and Mac."
test = "hermit --version"
binaries = ["hermit"]

rename = {
  "hermit-${os}": "hermit",
}

channel "canary" {
  source = "https://github.com/cashapp/hermit/releases/download/canary/hermit-${os}.gz"
  update = "24h"
}
