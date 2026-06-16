package nativemessaging

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	// msgBufferSize is the maximum message size we expect. Technically the extension is allowed to send a message
	// with size up to 64 MiB, but we restrict further.
	msgBufferSize      = 8192
	maxSendMessageSize = 1000000 // 1MB
)

// readMessage reads the incoming message from the msgReader.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-protocol
func readMessage(msgReader io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	headerBytesRead, err := io.ReadFull(msgReader, header)
	if err != nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return nil, fmt.Errorf("stream closed: %w", err)
	}
	if headerBytesRead != 4 || err != nil {
		return nil, fmt.Errorf("reading next header: %w", err)
	}

	msgLength := binary.NativeEndian.Uint32(header)
	if msgLength > msgBufferSize {
		return nil, fmt.Errorf("received message with length %d exceeding max %d", msgLength, msgBufferSize)
	}

	msgContentRaw := make([]byte, int(msgLength))
	msgBytesRead, err := io.ReadFull(msgReader, msgContentRaw)
	if err != nil {
		return nil, fmt.Errorf("reading message of length %d: %w", msgLength, err)
	}
	if msgBytesRead < int(msgLength) {
		return nil, fmt.Errorf("could only read %d of %d bytes in message", msgBytesRead, msgLength)
	}

	return msgContentRaw, nil
}

// sendMessage formats the given message body appropropriately
// and then writes it to stdout.
func sendMessage(msgBody any) error {
	msg, err := formatMessage(msgBody)
	if err != nil {
		return fmt.Errorf("formatting message: %w", err)
	}
	written, err := os.Stdout.Write(msg)
	if written != len(msg) || err != nil {
		return fmt.Errorf("sending message: wrote %d of %d expected bytes: %w", written, len(msg), err)
	}

	return nil
}

// formatMessage formats the given body by marshalling it to JSON and
// adding the expected header.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-protocol
func formatMessage(msgBody any) ([]byte, error) {
	bodyRaw, err := json.Marshal(msgBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling msg body to JSON: %w", err)
	}
	bodyLen := len(bodyRaw)
	totalMsgLen := bodyLen + 4
	if totalMsgLen > maxSendMessageSize {
		return nil, fmt.Errorf("message with size %d bytes exceeds max of 1 MB", totalMsgLen)
	}

	header := make([]byte, 4)
	binary.NativeEndian.PutUint32(header, uint32(bodyLen))

	return append(header, bodyRaw...), nil
}
