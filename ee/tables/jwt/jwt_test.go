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
			name: "rsa256 JWT",
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
