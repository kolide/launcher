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
		keys map[string]string
		raw  []string
	}{
		{
			name: "rsa256 JWT",
			path: "testdata/rsa256.raw",
			keys: map[string]string{
				"test": rsa256_key,
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
		})
	}
}
