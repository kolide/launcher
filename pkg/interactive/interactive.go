package interactive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/osquery/osquery-go"
)

const extensionName = "com.kolide.launcher_interactive"

func StartProcess(rootDir, osquerydPath string, osqueryFlags []string) (*os.Process, error) {

	if err := os.MkdirAll(rootDir, fs.DirMode); err != nil {
		return nil, fmt.Errorf("creating root dir for interactive mode: %w", err)
	}

	socketPath, err := socketPath(rootDir, osqueryFlags)
	if err != nil {
		return nil, fmt.Errorf("creating socket path: %w", err)
	}

	// Get the current working directory.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error getting current working directory: %w", err)
	}

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir:   cwd,
	}

	proc, err := os.StartProcess(osquerydPath, buildOsqueryFlags(socketPath, osqueryFlags), &pa)
	if err != nil {
		return nil, fmt.Errorf("error starting osqueryd in interactive mode: %w", err)
	}

	_, err = loadExtensions(socketPath, osquerydPath)
	if err != nil {
		return nil, fmt.Errorf("error loading extensions: %s", err)
	}

	return proc, nil
}

func buildOsqueryFlags(socketPath string, osqueryFlags []string) []string {

	// putting "-S" (the interactive flag) first because the behavior is inconsistent
	// when it's in the middle, found this during development on M1 macOS monterey 12.4
	// ~James Pickett 07/05/2022
	flags := []string{"-S"}

	for _, flag := range osqueryFlags {
		flags = append(flags, fmt.Sprintf("--%s", flag))
	}

	// required flags for interactive mode to function
	flags = append(flags, []string{
		"--disable_extensions=false",
		"--extensions_timeout=10",
		fmt.Sprintf("--extensions_require=%s", extensionName),
		fmt.Sprintf("--extensions_socket=%s", socketPath),
	}...)

	return flags
}

// MaxSocketPathCharacters is set to 97 because a ".12345" uuid is added to the socket down stream
// if the provided socket is greater than 97 we may exceed the limit of 103
// why 103 limit? https://unix.stackexchange.com/questions/367008/why-is-socket-path-length-limited-to-a-hundred-chars
const MaxSocketPathCharacters = 97

func socketPath(rootDir string, osqueryFlags []string) (string, error) {

	path := ""

	for _, flag := range osqueryFlags {
		if strings.HasPrefix(flag, "extensions_socket=") {
			split := strings.Split(flag, "=")
			if len(split) > 1 && split[1] != "" {
				path = split[1]
			} else {
				return "", errors.New("extensions_socket flag is missing a value")
			}
		}
	}

	if path == "" {
		path = filepath.Join(rootDir, "sock")
	}

	if len(path) > MaxSocketPathCharacters {
		return "", fmt.Errorf("socket path %s (%d characters) exceeded the maximum socket path character length of %d", path, len(path), MaxSocketPathCharacters)
	}

	return path, nil
}

func loadExtensions(socketPath string, osquerydPath string) (*osquery.ExtensionManagerServer, error) {

	extensionManagerServer, err := osquery.NewExtensionManagerServer(
		extensionName,
		socketPath,
		osquery.ServerTimeout(10*time.Second),
	)

	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating extension manager server: %s", err)
	}

	client, err := osquery.NewClient(socketPath, 10*time.Second)
	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating osquery client: %s", err)
	}

	extensionManagerServer.RegisterPlugin(table.PlatformTables(client, log.NewNopLogger(), osquerydPath)...)
	extensionManagerServer.RegisterPlugin(table.LauncherTables(nil, nil)...)

	if err := extensionManagerServer.Start(); err != nil {
		return nil, fmt.Errorf("error starting extension manager server: %s", err)
	}

	return extensionManagerServer, nil
}
