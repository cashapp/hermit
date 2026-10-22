package platform

import (
	"maps"
	"slices"
)

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
	// Glibc is the glibc version available on the platform, or zero if glibc is absent or unknown.
	Glibc GlibcVersion
}

func (p Platform) String() string {
	if p.Glibc.IsZero() {
		return p.OS + "-" + p.Arch
	}
	return p.OS + "-" + p.Arch + "-glibc" + p.Glibc.String()
}

// Base returns the platform's OS and architecture without any other values.
func (p Platform) Base() Platform {
	return Platform{OS: p.OS, Arch: p.Arch}
}

// Describe returns the values used to match the platform, eg. "os=linux arch=amd64 glibc=2.35".
func (p Platform) Describe() string {
	desc := "os=" + p.OS + " arch=" + p.Arch
	switch {
	case p.OS != Linux:
		return desc
	case p.Glibc.IsZero():
		return desc + " glibc=none"
	default:
		return desc + " glibc=" + p.Glibc.String()
	}
}

type Platforms []Platform

func (p Platforms) Arches() []string {
	arches := map[string]bool{}
	for _, platform := range p {
		arches[platform.Arch] = true
	}
	return slices.Collect(maps.Keys(arches))
}

func (p Platforms) OSes() []string {
	oses := map[string]bool{}
	for _, platform := range p {
		oses[platform.OS] = true
	}
	return slices.Collect(maps.Keys(oses))
}

// Core platforms officially supported by Hermit.
//
// For a package to be considered fully compliant, these platforms need to be supported
var Core = Platforms{
	{OS: Linux, Arch: Amd64},
	{OS: Darwin, Arch: Amd64},
	{OS: Darwin, Arch: Arm64},
}

// Optional platforms supported on a best-effort basis.
//
// Fully compliant packages are not required to support these platforms
var Optional = Platforms{
	{OS: Linux, Arch: Arm64},
}

// All platforms supported by Hermit.
var All = append(Core, Optional...)

var xarch = map[string]string{
	Amd64: "x86_64",
	"386": "i386",
	Arm64: "aarch64",
}

// ArchToXArch maps "arch" to "xarch".
func ArchToXArch(arch string) string {
	return xarch[arch]
}
