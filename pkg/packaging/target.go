package packaging

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Target is the platform being targeted by the build. As "platform"
// has several axis, we use a stuct to convey them.
type Target struct {
	Init     InitFlavor
	Package  PackageFlavor
	Platform PlatformFlavor
	Arch     ArchFlavor
}

type InitFlavor string

const (
	LaunchD          InitFlavor = "launchd"
	Systemd          InitFlavor = "systemd"
	Init             InitFlavor = "init"
	Upstart          InitFlavor = "upstart"
	WindowsService   InitFlavor = "service"
	NoInit           InitFlavor = "none"
	UpstartAmazonAMI InitFlavor = "upstart_amazon_ami"
)

var knownInitFlavors = [...]InitFlavor{LaunchD, Systemd, Init, Upstart, WindowsService, NoInit, UpstartAmazonAMI}

type PlatformFlavor string

const (
	Darwin  PlatformFlavor = "darwin"
	Windows PlatformFlavor = "windows"
	Linux   PlatformFlavor = "linux"
)

var knownPlatformFlavors = [...]PlatformFlavor{Darwin, Windows, Linux}

type PackageFlavor string

const (
	Pkg    PackageFlavor = "pkg"
	Tar    PackageFlavor = "tar"
	Deb    PackageFlavor = "deb"
	Rpm    PackageFlavor = "rpm"
	Msi    PackageFlavor = "msi"
	Pacman PackageFlavor = "pacman"
)

var knownPackageFlavors = [...]PackageFlavor{Pkg, Tar, Deb, Rpm, Msi, Pacman}

type ArchFlavor string

const (
	Arm64     ArchFlavor = "arm64"
	Amd64     ArchFlavor = "amd64"
	Universal ArchFlavor = "universal" // Darwin only
)

var defaultArchMap = map[PlatformFlavor]ArchFlavor{
	Darwin:  Universal,
	Windows: Amd64,
	Linux:   Amd64,
}

// Parse parses a string in the form platform-init-package and sets the target accordingly.
func (t *Target) Parse(s string) error {
	components := strings.Split(s, "-")
	if len(components) != 3 {
		return fmt.Errorf("unable to parse %s, should have exactly 3 components", s)
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

	// For now, set the default arch according to the given platform
	defaultArch, ok := defaultArchMap[t.Platform]
	if !ok {
		return fmt.Errorf("cannot select default arch for unknown platform %s", t.Platform)
	}
	t.Arch = defaultArch

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
	// Remove suffixes. This is order dependand, so slightly fragile.
	input = strings.TrimSuffix(input, ".ext")
	input = strings.TrimSuffix(input, ".exe")
	if t.Platform == Windows {
		return input + ".exe"
	} else {
		return input + ".ext"
	}
}

// PlatformBinaryName is a helper to return the platform specific binary suffix.
func (t *Target) PlatformBinaryName(input string) string {
	// remove trailing .exe
	input = strings.TrimSuffix(input, ".exe")

	if t.Platform == Windows {
		return input + ".exe"
	}
	return input
}

func (t *Target) PlatformLauncherPath(binDir string) string {
	if t.Platform == Darwin {
		// We want /usr/local/Kolide.app, not /usr/local/bin/Kolide.app, so we use Dir to strip out `bin`
		return filepath.Join(filepath.Dir(binDir), "Kolide.app", "Contents", "MacOS", "launcher")
	}

	return filepath.Join(binDir, t.PlatformBinaryName("launcher"))
}

// InitFromString sets a target's init flavor from string representation
func (t *Target) InitFromString(s string) error {
	for _, testInit := range knownInitFlavors {
		if testInit.String() == s {
			t.Init = testInit
			return nil
		}
	}
	return fmt.Errorf("unknown init %s", s)
}

// PlatformFromString sets a target's platform flavor from string representation
func (t *Target) PlatformFromString(s string) error {
	for _, testPlat := range knownPlatformFlavors {
		if testPlat.String() == s {
			t.Platform = testPlat
			return nil
		}
	}
	return fmt.Errorf("unknown platform %s", s)
}

// PackageFromString sets a target's package flavor from string representation
func (t *Target) PackageFromString(s string) error {
	for _, testPackage := range knownPackageFlavors {
		if testPackage.String() == s {
			t.Package = testPackage
			return nil
		}
	}
	return fmt.Errorf("unknown package %s", s)

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

func KnownInitFlavors() []string {
	out := make([]string, len(knownInitFlavors))
	for i, v := range knownInitFlavors {
		out[i] = v.String()
	}
	return out
}

func KnownPlatformFlavors() []string {
	out := make([]string, len(knownPlatformFlavors))
	for i, v := range knownPlatformFlavors {
		out[i] = v.String()
	}
	return out
}

func KnownPackageFlavors() []string {
	out := make([]string, len(knownPackageFlavors))
	for i, v := range knownPackageFlavors {
		out[i] = v.String()
	}
	return out
}
