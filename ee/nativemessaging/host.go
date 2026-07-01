package nativemessaging

import (
	"bufio"
	"context"
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
	for {
		msgContent, err := readMessage(stdinReader)
		if err != nil {
			// May be a genuine error, or may just be the stream closing
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				slogger.Log(ctx, slog.LevelInfo,
					"stream closed",
				)
			} else {
				slogger.Log(ctx, slog.LevelError,
					"terminating processing after error",
					"err", err,
				)
			}

			break
		}

		// In the future, we would JSON unmarshal this request and then forward it
		slogger.Log(ctx, slog.LevelInfo,
			"message",
			"contents", string(msgContent),
		)

		// Write a test message
		if err := sendMessage(map[string]any{
			"msg":     "received message",
			"msg_len": len(msgContent),
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

func ValidateNativeMessagingArgs(osArgs []string) (string, error) {
	// We always expect launcher to be called with 1 or 2 arguments, for both Firefox and Chrome
	// across all OSes.
	if len(osArgs) != 2 && len(osArgs) != 3 {
		return "", fmt.Errorf("unexpected number of args: expected 2 or 3, got %d", len(osArgs))
	}

	// For Chrome, launcher should be called with exactly 1 argument, which is the extension,
	// or with one additional argument on Windows:
	// `some-path-to/launcher chrome-extension://hjlinigoblmkhjejkmbegnoaljkphmgo/ --parent-window=0`.
	// We check for this case first, evaluating the first arg against our lookup map. We strip
	// the / suffix before performing the lookup against our known origins.
	potentialExtension := strings.TrimSuffix(osArgs[1], "/")
	if _, ok := allowlistedDt4aOriginsLookup[potentialExtension]; ok {
		return potentialExtension, nil
	}

	// For Firefox, launcher should be called with 1 or 2 arguments: the first argument
	// is the path to the app manifest, and the second argument (starting in Firefox 55)
	// is the extension ID. We explicitly do not support Firefox versions older than 55;
	// we want to be able to validate the extension ID.
	if len(osArgs) != 3 {
		return "", fmt.Errorf("extension does not match for Chrome, and wrong number of args for Firefox: %s", strings.Join(osArgs, ","))
	}
	// The extension ID in the args will be formatted like {0a75d802-9aed-41e7-8daa-24c067386e82};
	// update its format to moz-extension://0a75d802-9aed-41e7-8daa-24c067386e82 for the lookup.
	potentialExtension = fmt.Sprintf("moz-extension://%s", strings.TrimPrefix(strings.TrimSuffix(osArgs[2], "}"), "{"))
	if _, ok := allowlistedDt4aOriginsLookup[potentialExtension]; ok {
		return potentialExtension, nil
	}

	return "", fmt.Errorf("native messaging called with unexpected args: %s", strings.Join(osArgs, ","))
}

// validateNativeMessagingRequest validates that launcher has been launched by the expected process --
// Chrome, on behalf of a known extension.
func validateNativeMessagingRequest(ctx context.Context) (string, error) {
	potentialExtension, err := ValidateNativeMessagingArgs(os.Args)
	if err != nil {
		return "", fmt.Errorf("unexpected args for native messaging request: %w", err)
	}

	// Get the calling process so we can validate it
	browserProcess, browserPid, browserProcessName, err := getBrowserProcess(ctx)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting browser process: %w", err)
	}
	browserProcessCreateTime, err := browserProcess.CreateTimeWithContext(ctx)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting browser process create time for request from %s: %w", potentialExtension, err)
	}
	browserPath, err := browserProcess.ExeWithContext(ctx)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting executable for browser process: %w", err)
	}

	// Perform per-OS validation
	if err := validateBrowser(ctx, browserPath, browserProcessName); err != nil {
		return potentialExtension, fmt.Errorf("validating browser process %s: %w", browserProcessName, err)
	}

	// Check that the create time is still the same, so that we know the process hasn't died
	// and had its PID reused by some other process.
	browserProcessAfterValidation, err := process.NewProcessWithContext(ctx, browserPid)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting browser process after performing validation: %w", err)
	}
	browserProcessCreateTimeAfterValidation, err := browserProcessAfterValidation.CreateTimeWithContext(ctx)
	if err != nil {
		return potentialExtension, fmt.Errorf("getting browser process create time after performing validation: %w", err)
	}
	if browserProcessCreateTime != browserProcessCreateTimeAfterValidation {
		return potentialExtension, fmt.Errorf("PID reuse: browser process originally created at %d, now %d has create time at %d", browserProcessCreateTime, browserPid, browserProcessCreateTimeAfterValidation)
	}

	return potentialExtension, nil
}

// getBrowserProcess returns the process that ultimately created this process. Usually
// this is the parent process, but on Windows it can be one level above the parent.
func getBrowserProcess(ctx context.Context) (*process.Process, int32, string, error) {
	ppid := int32(os.Getppid())
	parentProcess, err := process.NewProcessWithContext(ctx, ppid)
	if err != nil {
		return nil, ppid, "", fmt.Errorf("getting parent process (%d): %w", ppid, err)
	}
	processName, err := parentProcess.NameWithContext(ctx)
	if err != nil {
		return nil, ppid, "", fmt.Errorf("getting name for parent process: %w", err)
	}

	// Some older versions of Chrome on Windows launch via cmd.exe, so we have to go up
	// one more level.
	if processName == "cmd.exe" {
		ppid, err = parentProcess.PpidWithContext(ctx)
		if err != nil {
			return nil, ppid, "", fmt.Errorf("getting cmd.exe parent process: %w", err)
		}
		parentProcess, err = process.NewProcessWithContext(ctx, ppid)
		if err != nil {
			return nil, ppid, "", fmt.Errorf("getting cmd.exe parent process (%d): %w", ppid, err)
		}

		processName, err = parentProcess.NameWithContext(ctx)
		if err != nil {
			return nil, ppid, "", fmt.Errorf("getting name for browser process: %w", err)
		}
	}

	return parentProcess, ppid, processName, nil
}
