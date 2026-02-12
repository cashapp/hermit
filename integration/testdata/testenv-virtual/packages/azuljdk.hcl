description = "Azul JDK - alternative jdk provider"
source = "${env}/packages/azuljdk.sh"
binaries = ["azuljdk.sh"]
env = {JAVA_HOME: "/path/to/azul"}
provides = ["jdk"]
version "21" {}
