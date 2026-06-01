package nativemessaging

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	agentsqlite "github.com/kolide/launcher/v2/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/v2/ee/localserver"
	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/peterbourgon/ff/v3"
	"github.com/shirou/gopsutil/v4/process"
)

// msgBufferSize is the maximum message size we expect. Technically the extension is allowed to send a message
// with size up to 64 MiB, but we restrict further.
const msgBufferSize = 8192

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
	// Set up a log writer to our kv.sqlite log store, if possible.
	// If not possible, we write logs to io.Discard instead.
	var logWriter io.WriteCloser
	var err error
	logWriter, err = agentsqlite.OpenRW(ctx, determineRootDirectory(), agentsqlite.NativeMessagingHostLogStore)
	if err != nil {
		logWriter = newNopCloser(io.Discard)
	}
	defer logWriter.Close()
	slogger := slog.New(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))

	extension, err := validateNativeMessagingRequest(ctx)
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
	if _, ok := localserver.AllowlistedDt4aOriginsLookup[potentialExtension]; !ok {
		return "", fmt.Errorf("native messaging called from unexpected extension %s", potentialExtension)
	}

	ppid := os.Getppid()
	parentProcess, err := process.NewProcessWithContext(ctx, int32(ppid))
	if err != nil {
		return "", fmt.Errorf("getting parent process (%d) for request from %s: %w", ppid, potentialExtension, err)
	}
	parentProcessExe, err := parentProcess.ExeWithContext(ctx)
	if err != nil {
		return "", fmt.Errorf("getting executable for parent process %d for request from %s: %w", ppid, potentialExtension, err)
	}

	if _, found := allowlistedChromePaths[parentProcessExe]; !found {
		return "", fmt.Errorf("path %s for ppid %d not in allowlisted chrome paths", parentProcessExe, ppid)
	}

	return potentialExtension, nil
}
