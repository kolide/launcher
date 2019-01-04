package packaging

import (
	"fmt"
	"strings"
)

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
	NoInit             = "none"
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

// Extension returns the extension that the resulting filesystem
// package should have. This may need to gain a PlatformFlavor in the
// future, and not just a straight string(PackageFlavor)
func (t *Target) PkgExtension() string {
	return strings.ToLower(string(t.Package))
}

// PlatformExtensionName is a helper to return the platform specific extension name.
func (t *Target) PlatformExtensionName(input string) string {
	if t.Platform == "Windows" {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

// PlatformBinaryName is a helper to return the platform specific binary suffix.
func (t *Target) PlatformBinaryName(input string) string {
	if t.Platform == "Windows" {
		return input + ".exe"
	}
	return input
}
