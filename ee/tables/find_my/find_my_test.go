//go:build darwin
// +build darwin

package find_my

import (
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFindMyDevice(t *testing.T) {
	t.Parallel()

	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up table
	findMyTable := FindMyDevice(mockFlags, slogger)

	// Query table
	response := findMyTable.Call(t.Context(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})

	// Confirm query worked
	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	require.Equal(t, 1, len(response.Response), "unexpected number of rows returned")
	require.Contains(t, response.Response[0], "find_my_mac_enabled")
}
