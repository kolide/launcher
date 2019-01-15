package packaging

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
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
	Upstart            = "upstart"
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

// Parse parses a string in the form platform-init-package and sets the target accordingly.
func (t *Target) Parse(s string) error {
	components := strings.Split(s, "-")
	if len(components) != 3 {
		return errors.Errorf("Unable to parse %s, should have exactly 3 components", s)
	}

	if err := t.PlatformFromString(components[0]); err != nil {
		return err
	}

	if err := t.InitFromString(components[1]); err != nil {
		return err
	}

	if err := t.PackageFromString(components[2]); err != nil {
		return err
	}

	return nil
}

// String returns the string representation
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
	if t.Platform == Windows {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

// PlatformBinaryName is a helper to return the platform specific binary suffix.
func (t *Target) PlatformBinaryName(input string) string {
	if t.Platform == Windows {
		return input + ".exe"
	}
	return input
}

// InitFromString sets a target's init flavor from string representation
func (t *Target) InitFromString(s string) error {
	for _, testInit := range []InitFlavor{LaunchD, SystemD, Init, Upstart, NoInit} {
		if testInit.String() == s {
			t.Init = testInit
			return nil
		}
	}
	return errors.Errorf("Unknown init %s", s)
}

// PlatformFromString sets a target's platform flavor from string representation
func (t *Target) PlatformFromString(s string) error {
	for _, testPlat := range []PlatformFlavor{Darwin, Windows, Linux} {
		if testPlat.String() == s {
			t.Platform = testPlat
			return nil
		}
	}
	return errors.Errorf("Unknown platform %s", s)
}

// PackageFromString sets a target's package flavor from string representation
func (t *Target) PackageFromString(s string) error {
	for _, testPackage := range []PackageFlavor{Pkg, Tar, Deb, Rpm, Msi} {
		if testPackage.String() == s {
			t.Package = testPackage
			return nil
		}
	}
	return errors.Errorf("Unknown package %s", s)

}

// String returns the string representation
func (i *InitFlavor) String() string {
	return strings.ToLower(string(*i))
}

// String returns the string representation
func (i *PlatformFlavor) String() string {
	return strings.ToLower(string(*i))
}

// String returns the string representation
func (i *PackageFlavor) String() string {
	return strings.ToLower(string(*i))
}
