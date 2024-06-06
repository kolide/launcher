package jwt

import (
	"context"
	_ "embed"
	"encoding/json"
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
				"signing_keys":    "{\"test\":\"-----BEGIN PUBLIC KEY-----\\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAu1SU1LfVLPHCozMxH2Mo\\n4lgOEePzNm0tRgeLezV6ffAt0gunVTLw7onLRnrq0/IzW7yWR7QkrmBL7jTKEn5u\\n+qKhbwKfBstIs+bMY2Zkp18gnTxKLxoS2tFczGkPLPgizskuemMghRniWaoLcyeh\\nkd3qqGElvW/VDL5AaWTg0nLVkjRo9z+40RQzuVaE8AkAFmxZzow3x+VJYKdjykkJ\\n0iT9wCS0DRTXu269V264Vf/3jvredZiKRkgwlL9xNAwxXFg0x/XFw005UWVRIkdg\\ncKWTjpBP2dPwVZ4WWC+9aGVd+Gyn1o0CLelf4rEjGoXbAAEgAqeGUxrcIlbjXfbc\\nmwIDAQAB\\n-----END PUBLIC KEY-----\"}",
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
				"signing_keys":    "{\"US2\":\"-----BEGIN PUBLIC KEY-----\\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEEVs/o5+uQbTjL3chynL4wXgUg2R9\\nq9UU8I5mEovUf86QZ7kOBIjJwqnzD1omageEHWwHdBO6B+dFabmdT9POxg==\\n-----END PUBLIC KEY-----\"}",
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
				"signing_keys":    "{\"blahblah\":\"-----BEGIN PUBLIC KEY-----\\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAu1SU1LfVLPHCozMxH2Mo\\n4lgOEePzNm0tRgeLezV6ffAt0gunVTLw7onLRnrq0/IzW7yWR7QkrmBL7jTKEn5u\\n+qKhbwKfBstIs+bMY2Zkp18gnTxKLxoS2tFczGkPLPgizskuemMghRniWaoLcyeh\\nkd3qqGElvW/VDL5AaWTg0nLVkjRo9z+40RQzuVaE8AkAFmxZzow3x+VJYKdjykkJ\\n0iT9wCS0DRTXu269V264Vf/3jvredZiKRkgwlL9xNAwxXFg0x/XFw005UWVRIkdg\\ncKWTjpBP2dPwVZ4WWC+9aGVd+Gyn1o0CLelf4rEjGoXbAAEgAqeGUxrcIlbjXfbc\\nmwIDAQAB\\n-----END PUBLIC KEY-----\"}",
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
			name: "rsa256 JWT unknown",
			path: "testdata/rsa256.raw",
			res: map[string]string{
				"parent":          "",
				"key":             "verified",
				"fullkey":         "verified",
				"value":           "",
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
			require.Contains(t, rows, tt.res, "generated rows should contain the expected result")
		})
	}
}
