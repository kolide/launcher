package nativemessaging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_formatMessage_readMessage(t *testing.T) {
	t.Parallel()

	// Create and format the message
	msg := map[string]any{
		"key": "value",
	}
	msgRaw, err := formatMessage(msg)
	require.NoError(t, err)

	// Write the message
	var buf bytes.Buffer
	written, err := buf.Write(msgRaw)
	require.NoError(t, err)
	require.Equal(t, len(msgRaw), written)

	// Now, read the message
	readRaw, err := readMessage(&buf)
	require.NoError(t, err)

	// Confirm output is same as input
	var read map[string]any
	require.NoError(t, json.Unmarshal(readRaw, &read))
	require.Equal(t, msg, read)
}
