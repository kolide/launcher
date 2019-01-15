package packaging

import (
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
			out: SystemD,
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

func TestPackageFromString(t *testing.T) {
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
	}

	// Test error case
	target := &Target{}
	err := target.PackageFromString("does not exist")
	require.Error(t, err)

	for _, tt := range tests {
		target := &Target{}
		err := target.PackageFromString(tt.in)
		require.NoError(t, err)
		require.Equal(t, string(tt.out), string(target.Package))
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
			out: &Target{Platform: Linux, Init: SystemD, Package: Rpm},
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
