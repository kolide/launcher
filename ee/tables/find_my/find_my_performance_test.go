//go:build darwin && performance
// +build darwin,performance

package find_my

import (
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
)

func TestFindMyDevice_MemoryImpact(t *testing.T) { //nolint:paralleltest
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up table
	findMyTable := FindMyDevice(mockFlags, slogger)

	queryCount := 100

	ci.AssessMemoryImpact(t, findMyTable, "{}", queryCount, true)
}
