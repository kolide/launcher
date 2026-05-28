package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"log/slog"
	"os"
)

// msgBufferSize is the maximum message size we expect. Technically the extension is allowed to send a message
// with size up to 64 MiB, but we restrict further.
const msgBufferSize = 8192

func readNativeMessages() {
	// For now, we just write received messages to a log file.
	logFile := `./native-logs.json`
	fh, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return
	}
	defer fh.Close()
	logger := slog.New(slog.NewJSONHandler(fh, nil))

	logger.Log(context.TODO(), slog.LevelInfo,
		"native messaging app opened",
		"args", os.Args,
	)

	stdinReader := bufio.NewReaderSize(bufio.NewReader(os.Stdin), msgBufferSize)
	header := make([]byte, 4)
	for {
		headerBytesRead, err := stdinReader.Read(header)
		if headerBytesRead == 0 || err != nil {
			logger.Log(context.TODO(), slog.LevelWarn,
				"could not read next header",
				"err", err,
			)
			break
		}

		msgLength := binary.NativeEndian.Uint32(header)

		logger.Log(context.TODO(), slog.LevelInfo,
			"received message with length",
			"length", msgLength,
		)

		msgContent := make([]byte, int(msgLength))
		msgBytesRead, err := stdinReader.Read(msgContent)
		if msgBytesRead < int(msgLength) {
			logger.Log(context.TODO(), slog.LevelWarn,
				"could not read all bytes in message",
				"length", msgLength,
				"bytes_read", msgBytesRead,
			)
			break
		} else if err != nil {
			logger.Log(context.TODO(), slog.LevelWarn,
				"could not read message",
				"err", err,
			)
			break
		}

		logger.Log(context.TODO(), slog.LevelWarn,
			"message",
			"contents", string(msgContent),
		)
	}

	logger.Log(context.TODO(), slog.LevelInfo,
		"shutting down",
	)
}
