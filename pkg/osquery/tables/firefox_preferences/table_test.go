package firefox_preferences

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_generateAirportData_HappyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filepath    string
		want        []map[string]string
		errContains string
	}{
		{
			name:     "happy path",
			filepath: "testdata/prefs.js",
			want: []map[string]string{
				{"app.normandy.first_run": "false"},
				{"app.normandy.migrationsApplied": "12"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			constraints := make(map[string][]string)

			constraints["path"] = append(constraints["path"], tt.filepath)

			got, err := generateData(tablehelpers.MockQueryContext(constraints), log.NewNopLogger())
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
