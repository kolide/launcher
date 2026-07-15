//go:build darwin

package pkgutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/pkgutil/mocks"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestGeneratePkgutilData(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name           string
		execReturnFile string
		want           []map[string]string
		assertion      assert.ErrorAssertionFunc
	}{
		{
			name:           "valid nonempty results",
			execReturnFile: "valid_nonempty.output",
			want: []map[string]string{
				{
					"package_id": "com.apple.pkg.CLTools_SDK_macOS13",
				},
				{
					"package_id": "org.golang.go",
				},
				{
					"package_id": "com.tinyspeck.slackmacgap",
				},
				{
					"package_id": "com.google.Chrome",
				},
			},
			assertion: assert.NoError,
		},
		{
			name:           "valid empty results",
			execReturnFile: "valid_empty.output",
			want:           []map[string]string{},
			assertion:      assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			execReturn, err := os.ReadFile(filepath.Join("testdata", tt.execReturnFile))
			require.NoError(t, err, "read exec return file")

			executor := &mocks.Executor{}

			executor.On("Exec").Return(execReturn, nil).Once()

			got, err := generatePkgutilData(t.Context(), executor, multislogger.NewNopLogger())
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
