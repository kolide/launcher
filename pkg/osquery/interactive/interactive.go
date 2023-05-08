package interactive

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/augeas"
	osqueryRuntime "github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/osquery/osquery-go"
)

const extensionName = "com.kolide.launcher_interactive"

func StartProcess(rootDir, osquerydPath string, osqueryFlags []string) (*os.Process, *osquery.ExtensionManagerServer, error) {

	if err := os.MkdirAll(rootDir, fsutil.DirMode); err != nil {
		return nil, nil, fmt.Errorf("creating root dir for interactive mode: %w", err)
	}

	socketPath := osqueryRuntime.SocketPath(rootDir)
	augeasLensesPath := filepath.Join(rootDir, "augeas-lenses")

	// only install augeas lenses on non-windows platforms
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(augeasLensesPath, fsutil.DirMode); err != nil {
			return nil, nil, fmt.Errorf("creating augeas lens dir: %w", err)
		}

		if err := augeas.InstallLenses(augeasLensesPath); err != nil {
			return nil, nil, fmt.Errorf("error installing augeas lenses: %w", err)
		}
	}

	proc, err := os.StartProcess(osquerydPath, buildOsqueryFlags(socketPath, augeasLensesPath, osqueryFlags), &os.ProcAttr{
		// Transfer stdin, stdout, and stderr to the new process
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})

	if err != nil {
		return nil, nil, fmt.Errorf("error starting osqueryd in interactive mode: %w", err)
	}

	// while developing for windows it was found that it will sometimes take osquey a while
	// to create the socket, so we wait for it to exist before continuing
	if err := waitForFile(socketPath, time.Second/4, time.Second*10); err != nil {

		procKillErr := proc.Kill()
		if procKillErr != nil {
			err = fmt.Errorf("error killing osqueryd interactive: %s: %w", procKillErr, err)
		}

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

func buildOsqueryFlags(socketPath, augeasLensesPath string, osqueryFlags []string) []string {

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
		"--extensions_timeout=20",
		fmt.Sprintf("--extensions_require=%s", extensionName),
		fmt.Sprintf("--extensions_socket=%s", socketPath),
	}...)

	// only install augeas lenses on non-windows platforms
	if runtime.GOOS != "windows" {
		flags = append(flags, fmt.Sprintf("--augeas_lenses=%s", augeasLensesPath))
	}

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

	if err := extensionManagerServer.Start(); err != nil {
		return nil, fmt.Errorf("error starting extension manager server: %w", err)
	}

	return extensionManagerServer, nil
}

// wait until file is present or timeout
func waitForFile(path string, interval, timeout time.Duration) error {
	intervalTicker := time.NewTicker(interval)
	defer intervalTicker.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	f := func() bool {
		_, err := os.Stat(path)
		return err == nil
	}

	if f() {
		return nil
	}

	for {
		select {
		case <-timeoutTimer.C:
			return fmt.Errorf("timeout waiting for file: %s", path)
		case <-intervalTicker.C:
			if f() {
				return nil
			}
		}
	}
}
