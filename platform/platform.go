package platform

// Amd64 architecture
const Amd64 = "amd64"

// Arm64 architecture
const Arm64 = "arm64"

// Linux OS
const Linux = "linux"

// Darwin OS
const Darwin = "darwin"

// Platform describes a system where a package can be installed
type Platform struct {
	// OS is the operating system of the platform
	OS string
	// Arch is the CPU architecture of the platform
	Arch string
}

func (p Platform) String() string {
	return p.OS + "-" + p.Arch
}

// Core platforms officially supported by Hermit.
//
// For a package to be considered fully compliant, these platforms need to be supported
var Core = []Platform{
	{Linux, Amd64},
	{Darwin, Amd64},
	{Darwin, Arm64},
}

var xarch = map[string]string{
	Amd64: "x86_64",
	"386": "i386",
	Arm64: "aarch64",
}

// ArchToXArch maps "arch" to "xarch".
func ArchToXArch(arch string) string {
	return xarch[arch]
}
