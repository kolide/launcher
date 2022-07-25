package interactive

import (
	"fmt"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/osquery/osquery-go"
)

const extensionName = "com.kolide.launcher_interactive"

func StartProcess(rootDir, osquerydPath string, osqueryFlags []string) (*os.Process, *osquery.ExtensionManagerServer, error) {

	if err := os.MkdirAll(rootDir, fs.DirMode); err != nil {
		return nil, nil, fmt.Errorf("creating root dir for interactive mode: %w", err)
	}

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	socketPath := runtime.SocketPath(rootDir)

	proc, err := os.StartProcess(osquerydPath, buildOsqueryFlags(socketPath, osqueryFlags), &pa)
	if err != nil {
		return nil, nil, fmt.Errorf("error starting osqueryd in interactive mode: %w", err)
	}

	// while developing for windows it was found that it will sometimes take osquey a while
	// to create the socket, so we wait for it to exist before continuing
	if err := waitForFile(socketPath, time.Second/4, time.Second*10); err != nil {
		return nil, nil, fmt.Errorf("error waiting for osquery to create socket: %w", err)
	}

	extensionServer, err := loadExtensions(socketPath, osquerydPath)
	if err != nil {
		err = fmt.Errorf("error loading extensions: %w", err)

		procKillErr := proc.Kill()
		if procKillErr != nil {
			err = fmt.Errorf("error killing osqueryd interactive: %s: %w", procKillErr, err)
		}

		return nil, nil, err
	}

	return proc, extensionServer, nil
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

func loadExtensions(socketPath string, osquerydPath string) (*osquery.ExtensionManagerServer, error) {

	extensionManagerServer, err := osquery.NewExtensionManagerServer(
		extensionName,
		socketPath,
		osquery.ServerTimeout(10*time.Second),
	)

	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating extension manager server: %w", err)
	}

	client, err := osquery.NewClient(socketPath, 10*time.Second)
	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating osquery client: %w", err)
	}

	extensionManagerServer.RegisterPlugin(table.PlatformTables(client, log.NewNopLogger(), osquerydPath)...)
	extensionManagerServer.RegisterPlugin(table.LauncherTables(nil, nil)...)

	if err := extensionManagerServer.Start(); err != nil {
		return nil, fmt.Errorf("error starting extension manager server: %w", err)
	}

	return extensionManagerServer, nil
}

// wait until file is present or timeout
func waitForFile(path string, interval, timeout time.Duration) error {
	intervalTimer := time.NewTimer(interval)
	defer intervalTimer.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	f := func() bool {
		_, err := os.Stat(path)
		return err == nil
	}

	if f() {
		return nil
	}

	select {
	case <-intervalTimer.C:
		if f() {
			return nil
		}
	case <-timeoutTimer.C:
		return fmt.Errorf("timeout waiting for file: %s", path)
	}

	return nil
}
