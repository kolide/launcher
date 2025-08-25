package packagekit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_fpmArch(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		f            fpmOptions
		expectedArch string
	}{
		{
			testCaseName: "amd64",
			f: fpmOptions{
				arch: "amd64",
			},
			expectedArch: "amd64",
		},
		{
			testCaseName: "arm64 - deb",
			f: fpmOptions{
				arch:       "arm64",
				outputType: Deb,
			},
			expectedArch: "arm64",
		},
		{
			testCaseName: "arm64 - rpm",
			f: fpmOptions{
				arch:       "arm64",
				outputType: RPM,
			},
			expectedArch: "aarch64",
		},
		{
			testCaseName: "arm64 - tar",
			f: fpmOptions{
				arch:       "arm64",
				outputType: Tar,
			},
			expectedArch: "arm64",
		},
		{
			testCaseName: "arm64 - pacman",
			f: fpmOptions{
				arch:       "arm64",
				outputType: Pacman,
			},
			expectedArch: "arm64",
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedArch, fpmArch(tt.f))
		})
	}
}
