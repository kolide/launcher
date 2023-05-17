package main

import (
	"runtime"
	"testing"

	"github.com/kolide/kit/version"
	"github.com/stretchr/testify/require"
)

func TestCheckupPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		os          string
		expectedErr bool
	}{
		{
			name:        "supported",
			os:          runtime.GOOS,
			expectedErr: false,
		},
		{
			name:        "unsupported",
			os:          "not-an-os",
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := checkupPlatform(tt.os)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckupVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		version     version.Info
		expectedErr bool
	}{
		{
			name:        "happy path",
			version:     version.Version(),
			expectedErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := checkupVersion(tt.version)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
