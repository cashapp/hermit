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

// Core platforms are core platforms officially supported by Hermit.
// For a package to be considered fully compliant, these platforms need to be supported
var Core = []Platform{
	{Linux, Amd64},
	{Darwin, Amd64},
	{Darwin, Arm64},
}
