package packaging

import "fmt"

// Target is the platform being targetted by the build. As "platform"
// has several axis, we use a stuct to convey them.
type Target struct {
	Init     InitFlavor
	Package  PackageFlavor
	Platform PlatformFlavor
}

type InitFlavor string

const (
	LaunchD InitFlavor = "launchd"
	SystemD            = "systemd"
	Init               = "init"
)

type PlatformFlavor string

const (
	Darwin  PlatformFlavor = "darwin"
	Windows                = "windows"
	Linux                  = "linux"
)

type PackageFlavor string

const (
	Pkg PackageFlavor = "pkg"
	Tar               = "tar"
	Deb               = "deb"
	Rpm               = "rpm"
	Msi               = "msi"
)

func (t *Target) String() string {
	return fmt.Sprintf("%s-%s-%s", t.Platform, t.Init, t.Package)
}

func (t *Target) Extension() string {
	switch t.Package {
	case Pkg:
		return "pkg"
	case Tar:
		return "tar"
	case Deb:
		return "deb"
	case Rpm:
		return "rpm"
	case Msi:
		return "msi"
	}
	return ""
}

// extBinary is a helper to return the platform specific extension name.
func (t *Target) ExtBinary(input string) string {
	if t.Platform == "Windows" {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

// platformBinary is a helper to return the platform specific binary suffix.
func (t *Target) PlatformBinary(input string) string {
	if t.Platform == "Windows" {
		return input + ".exe"
	}
	return input
}
