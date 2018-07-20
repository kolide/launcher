package manager

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

const (
	// how long to wait for the osquery extension socket to open
	socketOpenTimeout = 10 * time.Second

	// how often to try to open the osquery extension socket
	socketOpenInterval = 200 * time.Millisecond

	// how often to check the health of the instance
	healthCheckInterval = 60 * time.Second
)

// InstanceManager manages an instance of osqueryd
type InstanceManager struct {
	////////////////////////
	// Instance artifacts //
	////////////////////////
	ctx                    context.Context
	cancel                 context.CancelFunc
	cmd                    *exec.Cmd
	databasePath           string
	extensionManagerServer *osquery.ExtensionManagerServer
	extensionManagerClient *osquery.ExtensionManagerClient
	extensionPath          string
	extensionAutoloadPath  string
	pidfilePath            string
	rmRootDirectory        func()
	usingTempDir           bool

	/////////////////////////////////////////////
	// channels for communicating with osquery //
	/////////////////////////////////////////////

	// for signaling errors and exits of osqueryd
	osquerydStopped chan error

	// for signaling errors and exits of the extension server
	extensionServerStopped chan error

	//////////////////////////////////////////////
	// options that may have been set by caller //
	//////////////////////////////////////////////
	binaryPath            string
	configPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	extensionSocketPath   string
	logger                log.Logger
	loggerPluginFlag      string
	retries               uint
	rootDirectory         string
	stdout                io.Writer
	stderr                io.Writer
}

// New creates a new osquery instance manager based on the options provided
// For example, a more customized caller might do something like the following:
//
//   instance, err := manager.New(
//     ctx,
//     manager.WithOsquerydBinary("/usr/local/bin/osqueryd"),
//     manager.WithRootDirectory("/var/foobar"),
//     manager.WithOsqueryExtensionPlugin(config.NewPlugin("custom", custom.GenerateConfigs)),
//     manager.WithConfigPluginFlag("custom"),
//     manager.WithOsqueryExtensionPlugin(logger.NewPlugin("custom", custom.LogString)),
//     manager.WithOsqueryExtensionPlugin(tables.NewPlugin("foobar", custom.FoobarColumns, custom.FoobarGenerate)),
//   )
func New(ctx context.Context, opts ...InstanceManagerOption) (*InstanceManager, error) {
	// create a manager
	manager := &InstanceManager{
		logger:          log.NewNopLogger(),
		osquerydStopped: make(chan error, 1),
	}

	// set the context and cancel based on the given context
	manager.ctx, manager.cancel = context.WithCancel(ctx)

	// apply the options to the instance manager
	for _, opt := range opts {
		opt(manager)
	}

	// If the path of the osqueryd binary wasn't explicitly defined by the caller,
	// try to find it in the path.
	if manager.binaryPath == "" {
		path, err := exec.LookPath("osqueryd")
		if err != nil {
			return nil, errors.Wrap(err, "osqueryd not supplied and not found")
		}
		manager.binaryPath = path
	}

	// If the caller did not define the directory which all of the osquery file
	// artifacts should be stored in, use a temporary directory.
	if manager.rootDirectory == "" {
		rootDirectory, err := ioutil.TempDir("", "")
		if err != nil {
			return nil, errors.Wrap(err, "couldn't create temp directory for osquery instance")
		}
		manager.rootDirectory = rootDirectory
		manager.rmRootDirectory = func() { os.Remove(rootDirectory) }
		manager.usingTempDir = true
	}

	// Based on the root directory, calculate the file names of all of the
	// required osquery artifact files

	// Determine the path to the extension
	exPath, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "finding path of launcher executable")
	}
	manager.extensionPath = filepath.Join(filepath.Dir(exPath), "osquery-extension.ext")
	if _, err := os.Stat(manager.extensionPath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "extension path does not exist: %s", manager.extensionPath)
		}
		return nil, errors.Wrapf(err, "could not stat extension path")
	}

	// Determine the path to the extension socket
	if manager.extensionSocketPath == "" {
		manager.extensionSocketPath = filepath.Join(manager.rootDirectory, "osquery.sock")
	}

	// Write the autoload file
	manager.extensionAutoloadPath = filepath.Join(manager.rootDirectory, "osquery.autoload")
	if err := ioutil.WriteFile(manager.extensionAutoloadPath, []byte(manager.extensionPath), 0644); err != nil {
		return nil, errors.Wrap(err, "could not write osquery extension autoload file")
	}

	// If a config plugin has not been set by the caller, then it is likely
	// that the instance will just be used for executing queries, so we
	// will use a minimal config plugin that basically is a no-op.
	if manager.configPluginFlag == "" {
		generateConfigs := func(ctx context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		}
		manager.extensionPlugins = append(
			manager.extensionPlugins,
			config.NewPlugin("internal_noop", generateConfigs),
		)
		manager.configPluginFlag = "internal_noop"
	}

	// If a logger plugin has not been set by the caller, we set a logger
	// plugin that outputs logs to the default application logger.
	if manager.loggerPluginFlag == "" {
		logString := func(ctx context.Context, typ logger.LogType, logText string) error {
			return nil
		}
		manager.extensionPlugins = append(
			manager.extensionPlugins,
			logger.NewPlugin("internal_noop", logString),
		)
		manager.loggerPluginFlag = "internal_noop"
	}

	// If a distributed plugin has not been set by the caller, we set a
	// distributed plugin that returns no queries.
	if manager.distributedPluginFlag == "" {
		getQueries := func(ctx context.Context) (*distributed.GetQueriesResult, error) {
			return &distributed.GetQueriesResult{}, nil
		}
		writeResults := func(ctx context.Context, results []distributed.Result) error {
			return nil
		}
		manager.extensionPlugins = append(
			manager.extensionPlugins,
			distributed.NewPlugin("internal_noop", getQueries, writeResults),
		)
		manager.distributedPluginFlag = "internal_noop"
	}

	// Now that we have accepted options from the caller and/or determined what
	// they should be due to them not being set, we are ready to create and start
	// the *exec.Cmd instance that will run osqueryd.
	manager.cmd = exec.Command(
		manager.binaryPath,
		fmt.Sprintf("--pidfile=%s", manager.pidfilePath),
		fmt.Sprintf("--database_path=%s", manager.databasePath),
		fmt.Sprintf("--extensions_socket=%s", manager.extensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", manager.extensionAutoloadPath),
		fmt.Sprintf("--config_plugin=%s", manager.configPluginFlag),
		fmt.Sprintf("--logger_plugin=%s", manager.loggerPluginFlag),
		fmt.Sprintf("--distributed_plugin=%s", manager.distributedPluginFlag),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--config_refresh=10",
		"--host_identifier=uuid",
		"--force=true",
		"--disable_watchdog",
		"--utc",
	)
	if manager.stdout != nil {
		manager.cmd.Stdout = manager.stdout
	}
	if manager.stderr != nil {
		manager.cmd.Stderr = manager.stderr
	}

	// Assign a PGID that matches the PID. This lets us kill the entire process group later.
	manager.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return manager, nil
}

// Start the osqueryd instance manager
func (manager *InstanceManager) Start() error {
	// create a ticker to check health
	healthTicker := time.NewTicker(healthCheckInterval)
	defer healthTicker.Stop()

	// start the osquery process, process watcher, extension server,
	// extension client and register the extensions
	if err := manager.startOSQuery(); err != nil {
		return errors.Wrap(err, "starting osquery and extensions")
	}

	for {
		select {

		// handle the health check interval
		case <-healthTicker.C:

			// ping the extension manager server
			serverStatus, err := manager.extensionManagerServer.Ping()
			if err != nil {
				return errors.Wrap(err, "could not ping extension server")
			}

			// return an error if the status code wasn't okay
			if serverStatus.Code != 0 {
				return errors.Errorf("ping extension server returned %d: %s",
					serverStatus.Code,
					serverStatus.Message,
				)
			}

			// ping the extension manager client
			clientStatus, err := manager.extensionManagerClient.Ping()
			if err != nil {
				return errors.Wrap(err, "could not ping osquery extension client")
			}

			// return an error if the status code wasn't okay
			if clientStatus.Code != 0 {
				return errors.Errorf("ping extension client returned %d: %s",
					serverStatus.Code,
					serverStatus.Message,
				)
			}

		// Handle when osqueryd stopped for some reason
		case <-manager.osquerydStopped:
			// TODO: restart here

		// Handle shutdown
		case <-manager.ctx.Done():
			manager.cleanup()
			return nil
		}
	}
}

// Query sends a query to the osqueryd instance
func (manager *InstanceManager) Query(query string) ([]map[string]string, error) {
	//TODO: this
	return nil, nil
}

func (manager *InstanceManager) Restart() error {
	// TODO: this
	return nil
}

// Shutdown the instance manager
func (manager *InstanceManager) Shutdown() {
	manager.cancel()
}

// start osquery, the extension server, the extension server client, and register the extensions
func (manager *InstanceManager) startOSQuery() error {

	// start osqueryd instance
	if err := manager.cmd.Start(); err != nil {
		return errors.Wrap(err, "starting osqueryd process")
	}

	// start osqueryd process watcher
	go func() {
		if err := manager.cmd.Wait(); err != nil {
			manager.osquerydStopped <- errors.Wrap(err, "running osqueryd command")
			return
		}
		manager.osquerydStopped <- errors.New("osquery process exited")
	}()

	// TODO: create the extension manager server

	// TODO: create the extension manager client

	// TODO: register the extensions

	// start the extension manager server
	go func() {
		if err := manager.extensionManagerServer.Start(); err != nil {
			manager.extensionServerStopped <- errors.Wrap(err, "starting exension server")
		}
		manager.extensionServerStopped <- errors.New("exension manager server exited")
	}()

	return nil
}

// cleanup the osqueryd process tree, the extension manager, and any temp dirs
func (manager *InstanceManager) cleanup() {
	// kill osqueryd and children
	if err := syscall.Kill(-manager.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// TODO: refactor to not check the string
		if !strings.Contains(err.Error(), "process already finished") {
			level.Info(manager.logger).Log("msg", "killing osquery process", "err", err)
		}
	}

	// shutdown the extension manager server
	if err := manager.extensionManagerServer.Shutdown(); err != nil {
		level.Info(manager.logger).Log("msg", "shutting down extension server", "err", err)
	}

	// cleanup the temp dir
	if manager.usingTempDir && manager.rmRootDirectory != nil {
		manager.rmRootDirectory()
	}

	// log why we exited if there was an error in the context
	if err := manager.ctx.Err(); err != nil {
		level.Info(manager.logger).Log("msg", "shut down because of error", "err", err)
	}
}
