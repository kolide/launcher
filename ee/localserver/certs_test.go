package localserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_generateSelfSignedCert(t *testing.T) {
	t.Parallel()

	cert, err := generateSelfSignedCert(context.TODO())
	require.NoError(t, err, "expected no error generating cert")
	require.NotEmpty(t, cert.Certificate, "expected cert")
}
