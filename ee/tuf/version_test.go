package tuf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizePinnedVersion(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                     string
		pinnedVersion            string
		binary                   autoupdatableBinary
		expectedSanitizedVersion string
	}{
		{
			name:                     "osqueryd, valid version",
			pinnedVersion:            "5.10.0",
			binary:                   binaryOsqueryd,
			expectedSanitizedVersion: "5.10.0",
		},
		{
			name:                     "osqueryd, invalid version",
			pinnedVersion:            "version five point ten point zero, please",
			binary:                   binaryOsqueryd,
			expectedSanitizedVersion: "",
		},
		{
			name:                     "osqueryd, valid early version",
			pinnedVersion:            "1.0.0",
			binary:                   binaryOsqueryd,
			expectedSanitizedVersion: "1.0.0",
		},
		{
			name:                     "launcher, valid version",
			pinnedVersion:            "1.6.3",
			binary:                   binaryLauncher,
			expectedSanitizedVersion: "1.6.3",
		},
		{
			name:                     "launcher, invalid version",
			pinnedVersion:            "alpha",
			binary:                   binaryLauncher,
			expectedSanitizedVersion: "",
		},
		{
			name:                     "launcher, version too early",
			pinnedVersion:            "1.5.3",
			binary:                   binaryLauncher,
			expectedSanitizedVersion: "",
		},
		{
			name:                     "launcher, valid version at minimum",
			pinnedVersion:            pinnedLauncherVersionMinimum.Original(),
			binary:                   binaryLauncher,
			expectedSanitizedVersion: pinnedLauncherVersionMinimum.Original(),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expectedSanitizedVersion, SanitizePinnedVersion(tt.binary, tt.pinnedVersion))
		})
	}
}

func Test_launcherVersionSupportsTuf(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                string
		launcherVersion     string
		expectedError       bool
		expectedSupportsTuf bool
	}{
		{
			name:                "version supports TUF",
			launcherVersion:     "1.6.0",
			expectedError:       false,
			expectedSupportsTuf: true,
		},
		{
			name:                "version is TUF minimum, supports TUF",
			launcherVersion:     tufVersionMinimum.Original(),
			expectedError:       false,
			expectedSupportsTuf: true,
		},
		{
			name:                "version is too early to support TUF",
			launcherVersion:     "1.0.0",
			expectedError:       false,
			expectedSupportsTuf: false,
		},
		{
			name:                "version is invalid",
			launcherVersion:     "not a semver",
			expectedError:       true,
			expectedSupportsTuf: false,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			supportsTuf, err := launcherVersionSupportsTuf(tt.launcherVersion)
			if tt.expectedError {
				require.Error(t, err, "expected error checking if version supports TUF")
			} else {
				require.NoError(t, err, "expected no error checking if version supports TUF")
			}
			require.Equal(t, tt.expectedSupportsTuf, supportsTuf)
		})
	}
}

func Test_versionFromTarget(t *testing.T) {
	t.Parallel()

	for _, testVersion := range []struct {
		target          string
		binary          autoupdatableBinary
		operatingSystem string
		version         string
	}{
		{
			target:          "launcher/darwin/launcher-0.10.1.tar.gz",
			binary:          binaryLauncher,
			operatingSystem: "darwin",
			version:         "0.10.1",
		},
		{
			target:          "launcher/windows/launcher-1.13.5.tar.gz",
			binary:          binaryLauncher,
			operatingSystem: "windows",
			version:         "1.13.5",
		},
		{
			target:          "launcher/linux/launcher-0.13.5-40-gefdc582.tar.gz",
			binary:          binaryLauncher,
			operatingSystem: "linux",
			version:         "0.13.5-40-gefdc582",
		},
		{
			target:          "osqueryd/darwin/osqueryd-5.8.1.tar.gz",
			binary:          binaryOsqueryd,
			operatingSystem: "darwin",
			version:         "5.8.1",
		},
		{
			target:          "osqueryd/windows/osqueryd-0.8.1.tar.gz",
			binary:          binaryOsqueryd,
			operatingSystem: "windows",
			version:         "0.8.1",
		},
		{
			target:          "osqueryd/linux/osqueryd-5.8.2.tar.gz",
			binary:          binaryOsqueryd,
			operatingSystem: "linux",
			version:         "5.8.2",
		},
	} {
		require.Equal(t, testVersion.version, versionFromTarget(testVersion.binary, filepath.Base(testVersion.target)))
	}
}
