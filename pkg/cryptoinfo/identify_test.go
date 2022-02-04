package cryptoinfo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentify(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in               []string
		expectedCount    int
		expectedError    bool
		expectedSubjects []string
	}{
		{
			in:               []string{filepath.Join("testdata", "test_crt.pem")},
			expectedCount:    1,
			expectedSubjects: []string{"www.example.com"},
		},
		{
			in:               []string{filepath.Join("testdata", "test_crt.pem"), filepath.Join("testdata", "test_crt.pem")},
			expectedCount:    2,
			expectedSubjects: []string{"www.example.com", "www.example.com"},
		},
		{
			in:               []string{filepath.Join("testdata", "test_crt.der")},
			expectedCount:    1,
			expectedSubjects: []string{"www.example.com"},
		},
		{
			in:            []string{filepath.Join("testdata", "empty")},
			expectedCount: 0,
		},
		{
			in:            []string{filepath.Join("testdata", "sslcerts.pem")},
			expectedCount: 129,
			expectedSubjects: []string{
				"Autoridad de Certificacion Firmaprofesional CIF A62634068",
				"Chambers of Commerce Root - 2008",
				"Global Chambersign Root - 2008",
				"ACCVRAIZ1",
				"Actalis Authentication Root CA",
			},
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			in := []byte{}
			for _, file := range tt.in {
				fileBytes, err := os.ReadFile(file)
				require.NoError(t, err, "reading input %s for setup", file)
				in = bytes.Join([][]byte{in, fileBytes}, nil)
			}

			results, err := Identify(in)
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)

			for i, expectedSubject := range tt.expectedSubjects {
				cert, ok := results[i].Data.(*certExtract)
				require.True(t, ok, "type assert")
				assert.Equal(t, expectedSubject, cert.Subject.CommonName)
			}
		})
	}
}
