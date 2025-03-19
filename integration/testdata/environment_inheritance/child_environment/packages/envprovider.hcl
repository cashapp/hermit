description = "provides env vars"
source = "${env}/packages/envprovider.sh"
binaries = ["envprovider.sh"]
env = {VARIABLE: "${version}"}
version "1.0.0" "1.0.1" {}
