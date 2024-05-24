package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForceNoChunkedEncoding(t *testing.T) {
	t.Parallel()

	req := &http.Request{
		Method: "POST",
		Body:   io.NopCloser(bytes.NewBufferString("Hello World")),
	}

	// Check no ContentLength
	require.Equal(t, int64(0), req.ContentLength)

	forceNoChunkedEncoding(context.TODO(), req)

	// Check that we _now_ have ContentLength
	require.Equal(t, int64(11), req.ContentLength)

	// Check contents are still as expected
	content := &bytes.Buffer{}
	len, err := io.Copy(content, req.Body)
	require.NoError(t, err)
	require.Equal(t, int64(11), len)
	require.Equal(t, "Hello World", content.String())
}
