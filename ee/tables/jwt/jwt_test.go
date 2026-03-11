package jwt

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

//go:embed testdata/rsa256.pubkey
var rsa256_key string

//go:embed testdata/es256.pubkey
var es256_key string

//go:embed testdata/ps256.pubkey
var ps256_key string

//go:embed testdata/rsa256.raw
var rsa256_raw string

func TestTransformOutput(t *testing.T) {
	t.Parallel()

	jwtTable := &Table{slogger: multislogger.NewNopLogger()}

	var tests = []struct {
		name    string
		path    string
		rawData []string
		raw     []string
		keys    map[string]string
		res     map[string]string
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
				"raw_data":        "",
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
				"raw_data":        "",
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
				"raw_data":        "",
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
				"raw_data":        "",
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
				"raw_data":        "",
				"include_raw_jwt": "false",
				"signing_keys":    "null",
			},
		},
		{
			name: "rsa256 JWT include raw populates raw_data",
			path: "testdata/rsa256.raw",
			raw:  []string{"true"},
			res: map[string]string{
				"parent":          "header",
				"key":             "alg",
				"fullkey":         "header/alg",
				"value":           "RS256",
				"query":           "*",
				"path":            "testdata/rsa256.raw",
				"raw_data":        rsa256_raw,
				"include_raw_jwt": "true",
				"signing_keys":    "null",
			},
		},
		{
			name:    "rsa256 JWT via raw_data",
			rawData: []string{rsa256_raw},
			res: map[string]string{
				"parent":          "header",
				"key":             "alg",
				"fullkey":         "header/alg",
				"value":           "RS256",
				"query":           "*",
				"path":            "",
				"raw_data":        rsa256_raw,
				"include_raw_jwt": "false",
				"signing_keys":    "null",
			},
		},
		{
			name:    "rsa256 JWT via raw_data with verification",
			rawData: []string{rsa256_raw},
			keys: map[string]string{
				"test": rsa256_key,
			},
			res: map[string]string{
				"parent":          "",
				"key":             "verified",
				"fullkey":         "verified",
				"value":           "client_valid",
				"query":           "*",
				"path":            "",
				"raw_data":        rsa256_raw,
				"include_raw_jwt": "false",
				"signing_keys":    "{\"test\":" + strconv.Quote(rsa256_key) + "}",
			},
		},
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keyJSON, _ := json.Marshal(tt.keys)

			constraints := map[string][]string{
				"signing_keys":    {string(keyJSON)},
				"include_raw_jwt": tt.raw,
			}
			if tt.path != "" {
				constraints["path"] = []string{tt.path}
			}
			if len(tt.rawData) > 0 {
				constraints["raw_data"] = tt.rawData
			}
			mockQC := tablehelpers.MockQueryContext(constraints)

			rows, err := jwtTable.generate(t.Context(), mockQC)

			require.NoError(t, err)

			if tt.name == "empty token" {
				require.Nil(t, rows, "the result should be nil for an empty token")
			} else {
				require.Contains(t, rows, tt.res, "generated rows should contain the expected result")
			}
		})
	}
}

func TestGenerateDoesNotPanicWithoutToken(t *testing.T) {
	t.Parallel()
	jwtTable := &Table{slogger: multislogger.NewNopLogger()}
	mockQC := tablehelpers.MockQueryContext(map[string][]string{
		"path": {"testdata/sdf"},
	})

	rows, err := jwtTable.generate(t.Context(), mockQC)
	require.NoError(t, err)
	require.Empty(t, rows, "the result should be empty for a non-parseable token")
}

func TestGenerateRequiresPathOrRawData(t *testing.T) {
	t.Parallel()
	jwtTable := &Table{slogger: multislogger.NewNopLogger()}
	mockQC := tablehelpers.MockQueryContext(map[string][]string{})

	_, err := jwtTable.generate(t.Context(), mockQC)
	require.Error(t, err, "should error when neither path nor raw_data is specified")
}
