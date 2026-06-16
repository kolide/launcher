package nativemessaging

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/kit/env"
	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/peterbourgon/ff/v3"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	// msgBufferSize is the maximum message size we expect. Technically the extension is allowed to send a message
	// with size up to 64 MiB, but we restrict further.
	msgBufferSize      = 8192
	maxSendMessageSize = 1000000 // 1MB
)

// nopCloser wraps a Writer with a no-op Close function; we use it
// to wrap io.Discard so it can be an io.WriteCloser
type nopCloser struct {
	io.Writer
}

func newNopCloser(w io.Writer) io.WriteCloser {
	return nopCloser{w}
}

func (nopCloser) Close() error { return nil }

func ReadNativeMessages(ctx context.Context) {
	// Set up logging so that we can capture any errors that occur when processing messages.
	// We can't write to kv.sqlite (as the watchdog does) because this process
	// won't have sufficient permissions. For now, we write to a file in the desktop
	// directory. If that's not possible (i.e. on Windows there is no desktop directory),
	// we write logs to io.Discard instead. In the future, root launcher will create
	// an appropriate directory for logs when it calls WriteNativeMessagingManifest.
	var logWriter io.WriteCloser
	var err error
	logFile := filepath.Join(determineRootDirectory(), fmt.Sprintf("desktop_%d", os.Getuid()), "nativemessaging.log")
	logWriter, err = os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		logWriter = newNopCloser(io.Discard)
	}
	defer logWriter.Close()
	slogHandler := slog.NewJSONHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	slogger := multislogger.New(slogHandler)

	// Validate the request that started this process
	extension, err := validateNativeMessagingRequest(ctx)
	if err != nil {
		slogger.Log(ctx, slog.LevelError,
			"invalid native messaging request",
			"err", err,
			"extension", extension,
		)
		return
	}

	slogger.Log(ctx, slog.LevelInfo,
		"native messaging app opened",
		"extension", extension,
	)

	// Handle input until the connection is closed
	stdinReader := bufio.NewReaderSize(os.Stdin, msgBufferSize)
	header := make([]byte, 4)
	for {
		headerBytesRead, err := io.ReadFull(stdinReader, header)
		if err != nil && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			slogger.Log(ctx, slog.LevelInfo,
				"stream closed",
			)
			break
		}
		if headerBytesRead != 4 || err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"could not read next header",
				"err", err,
			)
			break
		}

		msgLength := binary.NativeEndian.Uint32(header)

		if msgLength > msgBufferSize {
			slogger.Log(ctx, slog.LevelInfo,
				"received message with length exceeding max, terminating processing",
				"length", msgLength,
				"max_length", msgBufferSize,
			)
			break
		}

		slogger.Log(ctx, slog.LevelInfo,
			"received message with length",
			"length", msgLength,
		)

		msgContent := make([]byte, int(msgLength))
		msgBytesRead, err := io.ReadFull(stdinReader, msgContent)
		if msgBytesRead < int(msgLength) {
			slogger.Log(ctx, slog.LevelWarn,
				"could not read all bytes in message",
				"length", msgLength,
				"bytes_read", msgBytesRead,
			)
			break
		} else if err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"could not read message",
				"err", err,
			)
			break
		}

		// In the future, we would forward this request
		slogger.Log(ctx, slog.LevelInfo,
			"message",
			"contents", string(msgContent),
		)

		// Write a test message
		if err := sendMessage(map[string]any{
			"msg":     "received message",
			"msg_len": msgLength,
		}); err != nil {
			slogger.Log(ctx, slog.LevelError,
				"sending message",
				"err", err,
			)
		}
	}

	slogger.Log(ctx, slog.LevelInfo,
		"shutting down",
	)
}

// determineRootDirectory discovers the root directory associated with this installation
// of launcher. It pulls the identifier from the current running executable, uses that to find
// the config path, and pulls the root directory from the config.
func determineRootDirectory() string {
	rootDir := launcher.DefaultRootDirectoryPath
	currentExecutable, err := os.Executable()
	if err != nil {
		return rootDir
	}

	identifier := extractIdentifierFromExecutable(currentExecutable)

	// We could probably assume the correct root directory given the identifier,
	// but just in case, we go through the config file to discover the configured
	// root directory.
	configFilePath := launcher.DefaultPath(launcher.ConfigFile)
	if identifier != launcher.DefaultLauncherIdentifier {
		configFilePath = strings.ReplaceAll(configFilePath, launcher.DefaultLauncherIdentifier, identifier)
		rootDir = strings.ReplaceAll(rootDir, launcher.DefaultLauncherIdentifier, identifier)
	}

	// Parse out only the root directory from the config file.
	cfgFileHandle, err := os.Open(configFilePath)
	if err != nil {
		return rootDir
	}
	defer cfgFileHandle.Close()
	_ = ff.PlainParser(cfgFileHandle, func(name, value string) error {
		switch name {
		case "root_directory":
			rootDir = value
		}
		return nil
	})

	return rootDir
}

// extractIdentifierFromExecutable pulls the identifier (e.g. kolide-k2) out of
// the path for the current running executable `executablePath`.
// We're either running out of the original install location (the bin directory)
// or out of the update directory (inside the root directory). On all OSes, all
// of these options should contain the identifier for this installation.
// We check this path to extract the identifier, which will allow us to determine
// the root directory location.
func extractIdentifierFromExecutable(executablePath string) string {
	identifier := launcher.DefaultLauncherIdentifier
	if strings.Contains(executablePath, identifier) {
		// Default identifier
		return identifier
	}

	// Assume that local paths use the kolide-nababe-k2 identifier, since we don't
	// have another way of determining it for them.
	if strings.Contains(executablePath, filepath.Join("launcher", "build")) && !env.Bool("LAUNCHER_FORCE_UPDATE_IN_BUILD", false) {
		return "kolide-nababe-k2"
	}

	// We have a custom identifier, taking the format `kolide-<id>-k2`
	_, afterIdentifierStart, foundIdentifierStart := strings.Cut(executablePath, "kolide-")
	if foundIdentifierStart {
		isolatedIdentifier, _, foundIdentifierEnd := strings.Cut(afterIdentifierStart, "-k2")
		if foundIdentifierEnd {
			identifier = fmt.Sprintf("kolide-%s-k2", isolatedIdentifier)
		}
	}
	return identifier
}

// validateNativeMessagingRequest validates that launcher has been launched by the expected process --
// Chrome, on behalf of a known extension.
func validateNativeMessagingRequest(ctx context.Context) (string, error) {
	// launcher should be called with exactly 1 argument, which is the extension.
	if len(os.Args) != 2 {
		return "", fmt.Errorf("unexpected number of args: expected 2, got %d", len(os.Args))
	}
	// The extension should be one that we know about. It will have an extra / at the end, which we remove
	// before performing the lookup against our known origins.
	potentialExtension := strings.TrimSuffix(os.Args[1], "/")
	if _, ok := allowlistedDt4aOriginsLookup[potentialExtension]; !ok {
		return "", fmt.Errorf("native messaging called from unexpected extension %s", potentialExtension)
	}

	// Get the calling process so we can validate it
	ppid := os.Getppid()
	parentProcess, err := process.NewProcessWithContext(ctx, int32(ppid))
	if err != nil {
		return potentialExtension, fmt.Errorf("getting parent process (%d) for request from %s: %w", ppid, potentialExtension, err)
	}
	parentProcessCreateTime, err := parentProcess.CreateTimeWithContext(ctx)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting parent process create time for request from %s: %w", potentialExtension, err)
	}

	// Perform per-OS validation
	if err := validateBrowser(ctx, parentProcess); err != nil {
		return potentialExtension, fmt.Errorf("validating browser process with ppid %d: %w", ppid, err)
	}

	// Check that the create time is still the same, so that we know the process hasn't died
	// and had its PID reused by some other process.
	parentProcessCreateTimeAfterValidation, err := parentProcess.CreateTimeWithContext(ctx)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting parent process create time after performing validation: %w", err)
	}
	if parentProcessCreateTime != parentProcessCreateTimeAfterValidation {
		return potentialExtension, fmt.Errorf("PID reuse: parent process originally created at %d, now %d has create time at %d", parentProcessCreateTime, ppid, parentProcessCreateTimeAfterValidation)
	}

	return potentialExtension, nil
}

// sendMessage formats the given body by marshalling it to JSON,
// adding the expected header, and writing it to stdout.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-protocol
func sendMessage(msgBody any) error {
	bodyRaw, err := json.Marshal(msgBody)
	if err != nil {
		return fmt.Errorf("marshalling msg body to JSON: %w", err)
	}
	bodyLen := len(bodyRaw)
	totalMsgLen := bodyLen + 4
	if totalMsgLen > maxSendMessageSize {
		return fmt.Errorf("message with size %d bytes exceeds max of 1 MB", totalMsgLen)
	}

	header := make([]byte, 4)
	binary.NativeEndian.PutUint32(header, uint32(bodyLen))

	written, err := os.Stdout.Write(append(header, bodyRaw...))
	if written != totalMsgLen || err != nil {
		return fmt.Errorf("sending message: wrote %d of %d expected bytes: %w", written, totalMsgLen, err)
	}

	return nil
}
