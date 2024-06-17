package jwt

import (
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/rsa256.pubkey
var rsa256_key string

//go:embed testdata/es256.pubkey
var es256_key string

//go:embed testdata/ps256.pubkey
var ps256_key string

func TestTransformOutput(t *testing.T) {
	t.Parallel()

	jwtTable := &Table{slogger: multislogger.NewNopLogger()}

	var tests = []struct {
		name string
		path string
		raw  []string
		keys map[string]string
		res  map[string]string
	}{
		{
			name: "empty token",
			path: "testdata/empty",
		},
		{
			name: "rsa256 JWT valid",
			path: "testdata/rsa256.raw",
			keys: map[string]string{
				"test": rsa256_key,
			},
			res: map[string]string{
				"parent":          "",
				"key":             "verified",
				"fullkey":         "verified",
				"value":           "client_valid",
				"query":           "*",
				"path":            "testdata/rsa256.raw",
				"include_raw_jwt": "false",
				"signing_keys":    "{\"test\":" + strconv.Quote(rsa256_key) + "}",
			},
		},
		{
			name: "es256 JWT valid",
			path: "testdata/es256.raw",
			keys: map[string]string{
				"US2": es256_key,
			},
			res: map[string]string{
				"parent":          "",
				"key":             "verified",
				"fullkey":         "verified",
				"value":           "client_valid",
				"query":           "*",
				"path":            "testdata/es256.raw",
				"include_raw_jwt": "false",
				"signing_keys":    "{\"US2\":" + strconv.Quote(es256_key) + "}",
			},
		},
		{
			name: "ps256 JWT valid",
			path: "testdata/ps256.raw",
			keys: map[string]string{
				"blahblah": ps256_key,
			},
			res: map[string]string{
				"parent":          "",
				"key":             "verified",
				"fullkey":         "verified",
				"value":           "client_valid",
				"query":           "*",
				"path":            "testdata/ps256.raw",
				"include_raw_jwt": "false",
				"signing_keys":    "{\"blahblah\":" + strconv.Quote(ps256_key) + "}",
			},
		},
		{
			name: "rsa256 JWT invalid",
			path: "testdata/rsa256.raw",
			keys: map[string]string{
				"test": "",
			},
			res: map[string]string{
				"parent":          "",
				"key":             "verified",
				"fullkey":         "verified",
				"value":           "invalid",
				"query":           "*",
				"path":            "testdata/rsa256.raw",
				"include_raw_jwt": "false",
				"signing_keys":    "{\"test\":\"\"}",
			},
		},
		{
			name: "rsa256 JWT unverified",
			path: "testdata/rsa256.raw",
			res: map[string]string{
				"parent":          "header",
				"key":             "alg",
				"fullkey":         "header/alg",
				"value":           "RS256",
				"query":           "*",
				"path":            "testdata/rsa256.raw",
				"include_raw_jwt": "false",
				"signing_keys":    "null",
			},
		},
		{
			name: "rsa256 JWT include raw",
			path: "testdata/rsa256.raw",
			raw:  []string{"true"},
			res: map[string]string{
				"parent":          "",
				"key":             "raw_jwt",
				"fullkey":         "raw_jwt",
				"value":           "eyJhbGciOiJSUzI1NiIsImtpZCI6InRlc3QiLCJ0eXAiOiJKV1QifQ.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.miA4Wn5C_uG7dEJYCeeNWk30kWTSREXAbPI55BX7OHtF4QHN4mFQOp3dE7cJO2H1VF0gN5ZuVLk5Pdz3J6sIZ9d-MAxwC7knmvOWvVZpOwOu7AglhQg6yPmktsEfJ6s13sMSGy11ChgmPGIwzKr08PVV1l-gKfsvpKTuMmNynyo44nZzyvk9fBkTEislWCKRvROHX0MYWmmrsb_V4PX1fRXKK2IaOZSEA1wnB1P_NS1YZdhW6nAfxpWrKwkKM3rCGuxdA9TYBcAYMHkET7VCbLcTD724J1XdtLfZVm8kPwQqek85f8tQnG9wQse8-gCahDQ0Pu4auLEYoXkOsJKbBQ",
				"query":           "*",
				"path":            "testdata/rsa256.raw",
				"include_raw_jwt": "true",
				"signing_keys":    "null",
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keyJSON, _ := json.Marshal(tt.keys)

			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"path":            {tt.path},
				"signing_keys":    {string(keyJSON)},
				"include_raw_jwt": tt.raw,
			})

			rows, err := jwtTable.generate(context.TODO(), mockQC)

			require.NoError(t, err)

			if tt.name == "empty token" {
				require.Nil(t, rows, "the result should be nil for an empty token")
			} else {
				require.Contains(t, rows, tt.res, "generated rows should contain the expected result")
			}
		})
	}
}
