package katc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_extractQueryTargets(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName            string
		query                   string
		expectedDbName          string
		expectedObjectStoreName string
		expectErr               bool
	}{
		{
			testCaseName:            "correctly formed query",
			query:                   "some_db.some_obj_store",
			expectedDbName:          "some_db",
			expectedObjectStoreName: "some_obj_store",
			expectErr:               false,
		},
		{
			testCaseName: "missing db name",
			query:        ".some_obj_store",
			expectErr:    true,
		},
		{
			testCaseName: "missing object store name",
			query:        "some_db.",
			expectErr:    true,
		},
		{
			testCaseName: "query missing separator",
			query:        "some_db some_obj_store",
			expectErr:    true,
		},
		{
			testCaseName: "query has too many components",
			query:        "some_db.some_obj_store.some_other_component",
			expectErr:    true,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			dbName, objStoreName, err := extractIndexeddbQueryTargets(tt.query)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedDbName, dbName)
				require.Equal(t, tt.expectedObjectStoreName, objStoreName)
			}
		})
	}
}
