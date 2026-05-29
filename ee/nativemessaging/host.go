package nativemessaging

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kolide/launcher/v2/ee/localserver"
	"github.com/shirou/gopsutil/v4/process"
)

// msgBufferSize is the maximum message size we expect. Technically the extension is allowed to send a message
// with size up to 64 MiB, but we restrict further.
const msgBufferSize = 8192

func ReadNativeMessages() {
	// For now, we just write received messages to a log file.
	logFile := `./native-logs.json`
	fh, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return
	}
	defer fh.Close()
	slogger := slog.New(slog.NewJSONHandler(fh, nil))

	extension, err := validateNativeMessagingRequest()
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"invalid native messaging request",
			"err", err,
		)
	}

	slogger.Log(context.TODO(), slog.LevelInfo,
		"native messaging app opened",
		"extension", extension,
	)

	stdinReader := bufio.NewReaderSize(bufio.NewReader(os.Stdin), msgBufferSize)
	header := make([]byte, 4)
	for {
		headerBytesRead, err := stdinReader.Read(header)
		if headerBytesRead == 0 || err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"could not read next header",
				"err", err,
			)
			break
		}

		msgLength := binary.NativeEndian.Uint32(header)

		slogger.Log(context.TODO(), slog.LevelInfo,
			"received message with length",
			"length", msgLength,
		)

		msgContent := make([]byte, int(msgLength))
		msgBytesRead, err := stdinReader.Read(msgContent)
		if msgBytesRead < int(msgLength) {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"could not read all bytes in message",
				"length", msgLength,
				"bytes_read", msgBytesRead,
			)
			break
		} else if err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"could not read message",
				"err", err,
			)
			break
		}

		slogger.Log(context.TODO(), slog.LevelWarn,
			"message",
			"contents", string(msgContent),
		)
	}

	slogger.Log(context.TODO(), slog.LevelInfo,
		"shutting down",
	)
}

// validateNativeMessagingRequest validates that launcher has been launched by the expected process --
// Chrome, on behalf of a known extension.
func validateNativeMessagingRequest() (string, error) {
	// launcher should be called with exactly 1 argument, which is the extension.
	if len(os.Args) != 2 {
		return "", fmt.Errorf("unexpected number of args: expected 2, got %d", len(os.Args))
	}
	// The extension should be one that we know about. It will have an extra / at the end, which we remove
	// before performing the lookup against our known origins.
	potentialExtension := strings.TrimSuffix(os.Args[1], "/")
	if _, ok := localserver.AllowlistedDt4aOriginsLookup[potentialExtension]; !ok {
		return "", fmt.Errorf("native messaging called from unexpected extension %s", potentialExtension)
	}

	ppid := os.Getppid()
	parentProcess, err := process.NewProcess(int32(ppid))
	if err != nil {
		return "", fmt.Errorf("getting parent process (%d) for request from %s: %w", ppid, potentialExtension, err)
	}
	parentProcessExe, err := parentProcess.Exe()
	if err != nil {
		return "", fmt.Errorf("getting executable for parent process %d for request from %s: %w", ppid, potentialExtension, err)
	}

	if _, found := allowlistedChromePaths[parentProcessExe]; !found {
		return "", fmt.Errorf("path %s for ppid %d not in allowlisted chrome paths", parentProcessExe, ppid)
	}

	return potentialExtension, nil
}
