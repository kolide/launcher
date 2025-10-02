//go:build windows
// +build windows

package tpmrunner

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpmutil/tbs"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/tpmrunner/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_tpmRunner_windows(t *testing.T) {
	t.Parallel()

	t.Run("handles no tpm in exectue", func(t *testing.T) {
		t.Parallel()

		tpmSignerCreatorMock := mocks.NewTpmSignerCreator(t)
		tpmRunner, err := New(t.Context(), multislogger.NewNopLogger(), inmemory.NewStore(), withTpmSignerCreator(tpmSignerCreatorMock))
		require.NoError(t, err)

		// we should never try again after getting TPMNotFound err
		tpmSignerCreatorMock.On("CreateKey").Return(nil, nil, tbs.ErrTPMNotFound).Once()

		go func() {
			// sleep long enough to get through 2 cycles of execute

			// "CreateKey" should only be called once
			time.Sleep(3 * time.Second)
			tpmRunner.Interrupt(errors.New("test"))
		}()

		require.NoError(t, tpmRunner.Execute())
		require.Nil(t, tpmRunner.Public())
	})

	t.Run("handles terminal errors Public() call", func(t *testing.T) {
		t.Parallel()

		tpmSignerCreatorMock := mocks.NewTpmSignerCreator(t)
		tpmRunner, err := New(t.Context(), multislogger.NewNopLogger(), inmemory.NewStore(), withTpmSignerCreator(tpmSignerCreatorMock))
		require.NoError(t, err)

		// we should never try again after getting TPMNotFound err
		tpmSignerCreatorMock.On("CreateKey").Return(nil, nil, tbs.ErrTPMNotFound).Once()

		// this is the only time "CreateKey" should be called
		require.Nil(t, tpmRunner.Public())

		go func() {
			// sleep long enough to get through 2 cycles of execute
			time.Sleep(3 * time.Second)
			tpmRunner.Interrupt(errors.New("test"))
		}()

		require.NoError(t, tpmRunner.Execute())
		require.Nil(t, tpmRunner.Public())
	})
}

func Test_isTerminalError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "tpm not found err",
			err:      tbs.ErrTPMNotFound,
			expected: true,
		},
		{
			name:     "integrity check failed",
			err:      tpm2.Error{Code: tpm2.RCIntegrity},
			expected: true,
		},
		{
			name:     "is not terminal error",
			err:      errors.New("not terminal"),
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, isTerminalTPMError(tt.err))
		})
	}
}
