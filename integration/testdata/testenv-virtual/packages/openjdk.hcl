description = "OpenJDK - provides jdk virtual package"
source = "${env}/packages/openjdk.sh"
binaries = ["openjdk.sh"]
env = {JAVA_HOME: "/path/to/java"}
provides = ["jdk"]
version "21" {}
