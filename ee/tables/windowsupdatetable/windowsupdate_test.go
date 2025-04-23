//go:build windows
// +build windows

package windowsupdatetable

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_MarshalUnmarshalQueryResults(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		results      QueryResults
	}{
		{
			testCaseName: "successful query",
			results: QueryResults{
				RawResults:      []byte("some query results"),
				Locale:          "_default",
				IsDefaultLocale: 1,
				ErrStr:          "",
			},
		},
		{
			testCaseName: "empty results, no error",
			results: QueryResults{
				RawResults:      []byte{},
				Locale:          "_default",
				IsDefaultLocale: 1,
				ErrStr:          "",
			},
		},

		{
			testCaseName: "error",
			results: QueryResults{
				ErrStr: errors.New("test error").Error(),
			},
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// We should be able to marshal the results
			rawQueryResults, err := json.Marshal(tt.results)
			require.NoError(t, err)

			// We should be able to unmarshal the marshalled results
			var unmarshalledResults QueryResults
			require.NoError(t, json.Unmarshal(rawQueryResults, &unmarshalledResults))

			// The data should be identical
			require.Equal(t, tt.results.RawResults, unmarshalledResults.RawResults)
			require.Equal(t, tt.results.Locale, unmarshalledResults.Locale)
			require.Equal(t, tt.results.IsDefaultLocale, unmarshalledResults.IsDefaultLocale)
			require.Equal(t, tt.results.ErrStr, unmarshalledResults.ErrStr)
		})
	}
}
