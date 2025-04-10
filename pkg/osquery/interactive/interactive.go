package interactive

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/startupsettings"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/augeas"
	osqueryRuntime "github.com/kolide/launcher/pkg/osquery/runtime"
	"github.com/kolide/launcher/pkg/osquery/table"
	osquery "github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/config"
)

const (
	extensionName           = "com.kolide.launcher_interactive"
	defaultConfigPluginName = "interactive_config"
)

func StartProcess(knapsack types.Knapsack, interactiveRootDir string) (*os.Process, *osquery.ExtensionManagerServer, error) {
	if err := os.MkdirAll(interactiveRootDir, fsutil.DirMode); err != nil {
		return nil, nil, fmt.Errorf("creating root dir for interactive mode: %w", err)
	}

	// We need a shorter ulid to avoid running into socket path length issues.
	socketId := ulid.New()
	truncatedSocketId := socketId[len(socketId)-4:]
	socketPath := osqueryRuntime.SocketPath(interactiveRootDir, truncatedSocketId)
	augeasLensesPath := filepath.Join(interactiveRootDir, "augeas-lenses")

	// only install augeas lenses on non-windows platforms
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(augeasLensesPath, fsutil.DirMode); err != nil {
			return nil, nil, fmt.Errorf("creating augeas lens dir: %w", err)
		}

		if err := augeas.InstallLenses(augeasLensesPath); err != nil {
			return nil, nil, fmt.Errorf("error installing augeas lenses: %w", err)
		}
	}

	// check to see if a config flag path was given,
	// we need to check this before loading the default config plugin,
	// passing 2 configs to osquery will result in an error
	haveConfigPathOsqFlag := false
	for _, flag := range knapsack.OsqueryFlags() {
		if strings.HasPrefix(flag, "config_path") {
			haveConfigPathOsqFlag = true
			break
		}
	}

	// start building list of osq plugins with the kolide tables
	osqPlugins := table.PlatformTables(knapsack, types.DefaultRegistrationID, knapsack.Slogger(), knapsack.OsquerydPath())
	osqPlugins = append(osqPlugins, table.KolideCustomAtcTables(knapsack, types.DefaultRegistrationID, knapsack.Slogger())...)

	osqueryFlags := knapsack.OsqueryFlags()
	// if we were not provided a config path flag, try to add default config
	if !haveConfigPathOsqFlag {
		// check to see if we can actually get a config plugin
		configPlugin, err := generateConfigPlugin(knapsack.Slogger(), knapsack.RootDirectory())
		if err != nil {
			knapsack.Slogger().Log(context.TODO(), slog.LevelDebug,
				"error creating config plugin",
				"err", err,
			)
		} else {
			osqPlugins = append(osqPlugins, configPlugin)
			osqueryFlags = append(osqueryFlags, fmt.Sprintf("config_plugin=%s", defaultConfigPluginName))
		}
	}

	proc, err := os.StartProcess(knapsack.OsquerydPath(), buildOsqueryFlags(socketPath, augeasLensesPath, osqueryFlags), &os.ProcAttr{
		// Transfer stdin, stdout, and stderr to the new process
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error starting osqueryd in interactive mode: %w", err)
	}

	knapsack.Slogger().Log(context.TODO(), slog.LevelDebug,
		"created osquery process",
		"pid", proc.Pid,
	)

	// while developing for windows it was found that it will sometimes take osquery a while
	// to create the socket, so we wait for it to exist before continuing
	if err := waitForFile(socketPath, time.Second/4, time.Second*10); err != nil {
		procKillErr := proc.Kill()
		if procKillErr != nil {
			err = fmt.Errorf("error killing osqueryd interactive: %s: %w", procKillErr, err)
		}

		return nil, nil, fmt.Errorf("error waiting for osquery to create socket: %w", err)
	}

	knapsack.Slogger().Log(context.TODO(), slog.LevelDebug,
		"osquery socket file created",
	)

	extensionServer, err := loadExtensions(knapsack.Slogger(), socketPath, osqPlugins...)
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

func loadExtensions(slogger *slog.Logger, socketPath string, plugins ...osquery.OsqueryPlugin) (*osquery.ExtensionManagerServer, error) {
	client, err := osquery.NewClient(socketPath, 10*time.Second, osquery.MaxWaitTime(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("error creating osquery client: %w", err)
	}

	slogger.Log(context.TODO(), slog.LevelDebug,
		"created osquery client",
	)

	extensionManagerServer, err := osquery.NewExtensionManagerServer(
		extensionName,
		socketPath,
		osquery.ServerTimeout(10*time.Second),
		osquery.WithClient(client),
	)
	if err != nil {
		return extensionManagerServer, fmt.Errorf("error creating extension manager server: %w", err)
	}

	slogger.Log(context.TODO(), slog.LevelDebug,
		"created osquery extension server",
	)

	extensionManagerServer.RegisterPlugin(plugins...)

	slogger.Log(context.TODO(), slog.LevelDebug,
		"registered plugins with server",
	)

	if err := extensionManagerServer.Start(); err != nil {
		return nil, fmt.Errorf("error starting extension manager server: %w", err)
	}

	slogger.Log(context.TODO(), slog.LevelDebug,
		"started osquery extension server",
	)

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

func generateConfigPlugin(slogger *slog.Logger, launcherDaemonRootDir string) (*config.Plugin, error) {
	r, err := startupsettings.OpenReader(context.TODO(), slogger, launcherDaemonRootDir)
	if err != nil {
		return nil, fmt.Errorf("error opening startup settings reader: %w", err)
	}
	defer r.Close()

	// Use the default registration's config
	atcConfigKey := storage.KeyByIdentifier([]byte("auto_table_construction"), storage.IdentifierTypeRegistration, []byte(types.DefaultRegistrationID))
	atcConfig, err := r.Get(string(atcConfigKey))
	if err != nil {
		return nil, fmt.Errorf("error getting auto_table_construction from startup settings: %w", err)
	}
	if atcConfig == "" {
		return nil, errors.New("auto_table_construction is not set in startup settings")
	}

	return config.NewPlugin(defaultConfigPluginName, func(ctx context.Context) (map[string]string, error) {
		return map[string]string{defaultConfigPluginName: atcConfig}, nil
	}), nil
}
