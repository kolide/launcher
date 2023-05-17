package main

import (
	"errors"
	"runtime"
	"testing"

	"github.com/kolide/kit/version"
	"github.com/stretchr/testify/require"
)

func TestRunCheckups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		checkups []*checkup
	}{
		{
			name: "successful checkups",
			checkups: []*checkup{
				{
					name: "do nothing",
					check: func() (string, error) {
						return "", nil
					},
				},
			},
		},
		{
			name: "failed checkup",
			checkups: []*checkup{
				{
					name: "do nothing",
					check: func() (string, error) {
						return "", nil
					},
				},
				{
					name: "return error",
					check: func() (string, error) {
						return "", errors.New("checkup error")
					},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runCheckups(tt.checkups)
		})
	}
}

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

func TestCheckupArch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		os          string
		expectedErr bool
	}{
		{
			name:        "supported",
			os:          runtime.GOARCH,
			expectedErr: false,
		},
		{
			name:        "unsupported",
			os:          "not-an-arch",
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := checkupArch(tt.os)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckupRootDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filepaths   []string
		expectedErr bool
	}{
		{
			name:        "present",
			filepaths:   []string{},
			expectedErr: false,
		},
		{
			name:        "not present",
			filepaths:   []string{},
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := checkupRootDir(tt.filepaths)
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
