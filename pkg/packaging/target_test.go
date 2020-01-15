package packaging

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitFromString(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out InitFlavor
	}{
		{
			in:  "launchd",
			out: LaunchD,
		},
		{
			in:  "systemd",
			out: Systemd,
		},
		{
			in:  "init",
			out: Init,
		},
		{
			in:  "upstart",
			out: Upstart,
		},
		{
			in:  "none",
			out: NoInit,
		},
	}

	// Test error case
	target := &Target{}
	err := target.InitFromString("does not exist")
	require.Error(t, err)

	for _, tt := range tests {
		target := &Target{}
		err := target.InitFromString(tt.in)
		require.NoError(t, err)
		require.Equal(t, string(tt.out), string(target.Init))
	}
}

func TestPlatformFromString(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out PlatformFlavor
	}{
		{
			in:  "darwin",
			out: Darwin,
		},
		{
			in:  "windows",
			out: Windows,
		},
		{
			in:  "linux",
			out: Linux,
		},
	}

	// Test error case
	target := &Target{}
	err := target.PlatformFromString("does not exist")
	require.Error(t, err)

	for _, tt := range tests {
		target := &Target{}
		err := target.PlatformFromString(tt.in)
		require.NoError(t, err)
		require.Equal(t, string(tt.out), string(target.Platform))
	}
}

func TestPackageStrings(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out PackageFlavor
	}{
		{
			in:  "pkg",
			out: Pkg,
		},
		{
			in:  "tar",
			out: Tar,
		},
		{
			in:  "deb",
			out: Deb,
		},
		{
			in:  "rpm",
			out: Rpm,
		},
		{
			in:  "msi",
			out: Msi,
		},
		{
			in:  "pacman",
			out: "Pacman",
		},
	}

	// Test error case
	target := &Target{}
	err := target.PackageFromString("does not exist")
	require.Error(t, err)

	for _, tt := range tests {
		target := &Target{}
		err := target.PackageFromString(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.out, target.Package)

		// Check the reversal as well.
		require.Equal(t, tt.in, target.PkgExtension())
	}
}

func TestTargetParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in         string
		out        *Target
		shouldFail bool
	}{
		{
			in:  "darwin-launchd-pkg",
			out: &Target{Platform: Darwin, Init: LaunchD, Package: Pkg},
		},
		{
			in:  "windows-none-msi",
			out: &Target{Platform: Windows, Init: NoInit, Package: Msi},
		},
		{
			in:  "linux-systemd-rpm",
			out: &Target{Platform: Linux, Init: Systemd, Package: Rpm},
		},
		{
			in:         "windows-msi",
			shouldFail: true,
		},
		{
			in:         "does-not-exist",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		target := &Target{}
		err := target.Parse(tt.in)
		if tt.shouldFail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tt.out.String(), target.String())
		}
	}
}

func TestTargetPlatformBinaryName(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in     string
		out    string
		outwin string
	}{
		{
			in:     "foo",
			out:    "foo",
			outwin: "foo.exe",
		},
		{
			in:     "foo.app",
			out:    "foo.app",
			outwin: "foo.app.exe",
		},
		{
			in:     "foo.exe",
			out:    "foo",
			outwin: "foo.exe",
		},
	}

	target := &Target{Platform: Darwin, Init: LaunchD, Package: Pkg}
	targetWin := &Target{Platform: Windows, Init: NoInit, Package: Msi}

	for _, tt := range tests {
		require.Equal(t, tt.out, target.PlatformBinaryName(tt.in), fmt.Sprintf("Test: %s", tt.in))
		require.Equal(t, tt.outwin, targetWin.PlatformBinaryName(tt.in), fmt.Sprintf("Test: %s", tt.in))
	}
}
func TestTargetPlatformExtensionName(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in     string
		out    string
		outwin string
	}{
		{
			in:     "foo",
			out:    "foo.ext",
			outwin: "foo.exe",
		},
		{
			in:     "foo.ext",
			out:    "foo.ext",
			outwin: "foo.exe",
		},
		{
			in:     "foo.exe",
			out:    "foo.ext",
			outwin: "foo.exe",
		},
	}

	target := &Target{Platform: Darwin, Init: LaunchD, Package: Pkg}
	targetWin := &Target{Platform: Windows, Init: NoInit, Package: Msi}

	for _, tt := range tests {
		require.Equal(t, tt.out, target.PlatformExtensionName(tt.in), fmt.Sprintf("Test: %s", tt.in))
		require.Equal(t, tt.outwin, targetWin.PlatformExtensionName(tt.in), tt.in, fmt.Sprintf("Test: %s", tt.in))
	}

}
