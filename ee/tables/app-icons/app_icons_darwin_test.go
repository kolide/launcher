//go:build darwin
// +build darwin

package appicons

import (
	"fmt"
	"os"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
)

func Test_AppIcons_MemoryImpact(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("test not supported in CI, only running standalone")
	}

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	appIconsTable := AppIcons(mockFlags, multislogger.NewNopLogger())
	queryCount := 100

	paths := []string{
		"/Applications/1Password.app/Contents/Library/LoginItems/1Password Browser Helper.app",
		"not a real path",
		"/Applications/1Password.app/Contents/Library/LoginItems/1Password Launcher.app",
	}

	for _, p := range paths {
		// Set up our query
		queryContext := fmt.Sprintf(`{
	"constraints": [
		{
			"name": "path",
			"list": [
				{
					"op": 2,
					"expr": "%s"
				}
			]
		}
	]
}`, p)
		ci.AssessMemoryImpact(t, appIconsTable, queryContext, queryCount, true)
	}
}
