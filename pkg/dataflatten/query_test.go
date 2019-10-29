package dataflatten

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in        string
		query     []string
		out       string
		comments  string
		err       bool
		errString string
	}{
		/*
			{
				in:  `["a", "b", "c"]`,
				out: `[a b c]`,
			},
			{
				in:        `["a", "b", "c"]`,
				query:     []string{"a"},
				errString: "Not yet",
			},
		*/
		{
			in:    `{ "data": ["a", "b", "c"] }`,
			out:   `[a b c]`,
			query: []string{"data"},
		},
		{
			in:    `{"record": { "id": 1, "data": ["a", "b", "c"] } }`,
			out:   `[a b c]`,
			query: []string{"record", "data"},
		},
		{
			in:    `{"record": { "id": 1, "data": ["a", "b", "c"] } }`,
			out:   `a`,
			query: []string{"record", "data", "0"},
		},
		{
			in: `{ "records": [
                                            { "id": 1, "uuid": "abc123", "name": "Alice", "favs": [ "a", "b", "c" ] },
                                            { "id": 2, "uuid": "def456", "name": "Bob", "favs": [ "d", "e", "f" ] },
                                            { "id": 3, "uuid": "ghi789", "name": "Carol", "favs": [ "g", "h", "i" ] }
                                          ]}`,
			query: []string{"records", "uuid=>abc123", "favs", "1"},
			out:   "b",
		},
		{
			in: `{ "records": [
                                            { "id": 1, "uuid": "abc123", "name": "Alice", "favs": [ "a", "b", "c" ] },
                                            { "id": 2, "uuid": "def456", "name": "Bob", "favs": [ "d", "e", "f" ] },
                                            { "id": 3, "uuid": "ghi789", "name": "Carol", "favs": [ "g", "h", "i" ] }
                                          ]}`,
			query: []string{"records", "name=>Bob", "favs", "1"},
			out:   "e",
		},
		{
			in: `{ "records": [
                                            { "id": 1, "uuid": "abc123", "name": "Alice", "favs": [ "a", "b", "c" ] },
                                            { "id": 2, "uuid": "def456", "name": "Bob", "favs": [ "d", "e", "f" ] },
                                            { "id": 3, "uuid": "ghi789", "name": "Carol", "favs": [ "g", "h", "i" ] }
                                          ]}`,
			query: []string{"records", "id=>3", "favs", "1"},
			out:   "h",
		},
	}

	for _, tt := range tests {
		actual, err := QueryJson([]byte(tt.in), tt.query)
		if tt.err || tt.errString != "" {
			require.Error(t, err, tt.comments)
			if tt.errString != "" {
				require.Error(t, err, tt.errString, tt.comments)
			}
			return
		}

		require.NoError(t, err, tt.comments)
		require.EqualValues(t, tt.out, actual, tt.comments)
	}
}
