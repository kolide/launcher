package runtime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go"

	"golang.org/x/sync/errgroup"
)

// OsqueryInstanceOption is a functional option pattern for defining how an
// osqueryd instance should be configured. For more information on this pattern,
// see the following blog post:
// https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
type OsqueryInstanceOption func(*OsqueryInstance)

// WithOsqueryExtensionPlugins is a functional option which allows the user to
// declare a number of osquery plugins (ie: config plugin, logger plugin, tables,
// etc) which can be loaded when calling LaunchOsqueryInstance. You can load as
// many plugins as you'd like.
func WithOsqueryExtensionPlugins(plugins ...osquery.OsqueryPlugin) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.extensionPlugins = append(i.opts.extensionPlugins, plugins...)
	}
}

// WithRootDirectory is a functional option which allows the user to define the
// path where filesystem artifacts will be stored. This may include pidfiles,
// RocksDB database files, etc. If this is not defined, a temporary directory
// will be used.
func WithRootDirectory(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.rootDirectory = path
	}
}

func WithUpdateDirectory(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.updateDirectory = path
	}
}

func WithUpdateChannel(channel string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.updateChannel = channel
	}
}

// WithExtensionSocketPath is a functional option which allows the user to
// define the path of the extension socket path that osqueryd will open to
// communicate with other processes.
func WithExtensionSocketPath(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.extensionSocketPath = path
	}
}

// WithConfigPluginFlag is a functional option which allows the user to define
// which config plugin osqueryd should use to retrieve the config. If this is not
// defined, it is assumed that no configuration is needed and a no-op config
// will be used. This should only be configured once and cannot be changed once
// osqueryd is running.
func WithConfigPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.configPluginFlag = plugin
	}
}

// WithLoggerPluginFlag is a functional option which allows the user to define
// which logger plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default logger. The
// logger plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithLoggerPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.loggerPluginFlag = plugin
	}
}

// WithDistributedPluginFlag is a functional option which allows the user to define
// which distributed plugin osqueryd should use to log status and result logs. If this
// is not defined, logs will be logged via the application's default distributed. The
// distributed plugin which osquery uses can be changed at any point during the
// osqueryd execution lifecycle by defining the option via the config.
func WithDistributedPluginFlag(plugin string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.distributedPluginFlag = plugin
	}
}

// WithStdout is a functional option which allows the user to define where the
// stdout of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStdout(w io.Writer) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.stdout = w
	}
}

// WithStderr is a functional option which allows the user to define where the
// stderr of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStderr(w io.Writer) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.stderr = w
	}
}

// WithSlogger is a functional option which allows the user to pass a *slog.Logger
// to be used for logging osquery instance status.
func WithSlogger(slogger *slog.Logger) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.slogger = slogger
	}
}

// WithOsqueryVerbose sets whether or not osquery is in verbose mode
func WithOsqueryVerbose(v bool) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.verbose = v
	}
}

func WithEnrollSecretPath(secretPath string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.enrollSecretPath = secretPath
	}
}

func WithTlsHostname(hostname string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsHostname = hostname
	}
}

func WithTlsConfigEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsConfigEndpoint = ep
	}
}

func WithTlsEnrollEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsEnrollEndpoint = ep
	}
}

func WithTlsLoggerEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsLoggerEndpoint = ep
	}
}

func WithTlsDistributedReadEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsDistReadEndpoint = ep
	}
}

func WithTlsDistributedWriteEndpoint(ep string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsDistWriteEndpoint = ep
	}
}

func WithTlsServerCerts(s string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.tlsServerCerts = s
	}
}

// WithOsqueryFlags sets additional flags to pass to osquery
func WithOsqueryFlags(flags []string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.osqueryFlags = flags
	}
}

// WithAugeasLensFunction defines a callback function. This can be
// used during setup to populate the augeas lenses directory.
func WithAugeasLensFunction(f func(dir string) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.augeasLensFunc = f
	}
}

func WithKnapsack(k types.Knapsack) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.knapsack = k
	}
}

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	opts    osqueryOptions
	slogger *slog.Logger
	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	errgroup                *errgroup.Group
	doneCtx                 context.Context // nolint:containedctx
	cancel                  context.CancelFunc
	cmd                     *exec.Cmd
	emsLock                 sync.RWMutex // Lock for extensionManagerServers
	extensionManagerServers []*osquery.ExtensionManagerServer
	extensionManagerClient  *osquery.ExtensionManagerClient
	rmRootDirectory         func()
	usingTempDir            bool
	stats                   *history.Instance
	startFunc               func(cmd *exec.Cmd) error
	knapsack                types.Knapsack
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy. If the instance is healthy, it returns nil.
func (o *OsqueryInstance) Healthy() error {
	// Do not add/remove servers from o.extensionManagerServers while we're accessing them
	o.emsLock.RLock()
	defer o.emsLock.RUnlock()

	if len(o.extensionManagerServers) == 0 || o.extensionManagerClient == nil {
		return errors.New("instance not started")
	}

	for _, srv := range o.extensionManagerServers {
		serverStatus, err := srv.Ping(context.TODO())
		if err != nil {
			return fmt.Errorf("could not ping extension server: %w", err)
		}
		if serverStatus.Code != 0 {
			return fmt.Errorf("ping extension server returned %d: %s",
				serverStatus.Code,
				serverStatus.Message)

		}
	}

	clientStatus, err := o.extensionManagerClient.Ping()
	if err != nil {
		return fmt.Errorf("could not ping osquery extension client: %w", err)
	}
	if clientStatus.Code != 0 {
		return fmt.Errorf("ping extension client returned %d: %s",
			clientStatus.Code,
			clientStatus.Message)

	}

	return nil
}

func (o *OsqueryInstance) Query(query string) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(context.Background())
	defer span.End()

	if o.extensionManagerClient == nil {
		return nil, errors.New("client not ready")
	}

	resp, err := o.extensionManagerClient.QueryContext(ctx, query)
	if err != nil {
		traces.SetError(span, err)
		return nil, fmt.Errorf("could not query the extension manager client: %w", err)
	}
	if resp.Status.Code != int32(0) {
		traces.SetError(span, errors.New(resp.Status.Message))
		return nil, errors.New(resp.Status.Message)
	}

	return resp.Response, nil
}

type osqueryOptions struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	augeasLensFunc        func(dir string) error
	configPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	extensionSocketPath   string
	enrollSecretPath      string
	loggerPluginFlag      string
	osqueryFlags          []string
	rootDirectory         string
	stderr                io.Writer
	stdout                io.Writer
	tlsConfigEndpoint     string
	tlsDistReadEndpoint   string
	tlsDistWriteEndpoint  string
	tlsEnrollEndpoint     string
	tlsHostname           string
	tlsLoggerEndpoint     string
	tlsServerCerts        string
	updateChannel         string
	updateDirectory       string
	verbose               bool
}

// requiredExtensions returns a unique list of external
// extensions. These are extensions we expect osquery to pause start
// for.
func (o osqueryOptions) requiredExtensions() []string {
	extensionsMap := make(map[string]bool)
	requiredExtensions := make([]string, 0)

	for _, extension := range []string{o.loggerPluginFlag, o.configPluginFlag, o.distributedPluginFlag} {
		// skip the osquery build-ins, since requiring them will cause osquery to needlessly wait.
		if extension == "tls" {
			continue
		}

		if _, ok := extensionsMap[extension]; ok {
			continue
		}

		extensionsMap[extension] = true
		requiredExtensions = append(requiredExtensions, extension)
	}

	return requiredExtensions
}

func newInstance() *OsqueryInstance {
	i := &OsqueryInstance{}

	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel
	i.errgroup, i.doneCtx = errgroup.WithContext(ctx)

	i.slogger = multislogger.NewNopLogger()

	i.startFunc = func(cmd *exec.Cmd) error {
		return cmd.Start()
	}

	return i
}

// osqueryFilePaths is a struct which contains the relevant file paths needed to
// launch an osqueryd instance.
type osqueryFilePaths struct {
	augeasPath            string
	databasePath          string
	extensionAutoloadPath string
	extensionSocketPath   string
	pidfilePath           string
}

// calculateOsqueryPaths accepts a path to a working osqueryd binary and a root
// directory where all of the osquery filesystem artifacts should be stored.
// In return, a structure of paths is returned that can be used to launch an
// osqueryd instance. An error may be returned if the supplied parameters are
// unacceptable.
func calculateOsqueryPaths(opts osqueryOptions) (*osqueryFilePaths, error) {

	// Determine the path to the extension socket
	extensionSocketPath := opts.extensionSocketPath
	if extensionSocketPath == "" {
		extensionSocketPath = SocketPath(opts.rootDirectory)
	}

	extensionAutoloadPath := filepath.Join(opts.rootDirectory, "osquery.autoload")

	// We want to use a unique pidfile per launcher run to avoid file locking issues.
	// See: https://github.com/kolide/launcher/issues/1599
	osqueryFilePaths := &osqueryFilePaths{
		pidfilePath:           filepath.Join(opts.rootDirectory, fmt.Sprintf("osquery-%s.pid", ulid.New())),
		databasePath:          filepath.Join(opts.rootDirectory, "osquery.db"),
		augeasPath:            filepath.Join(opts.rootDirectory, "augeas-lenses"),
		extensionSocketPath:   extensionSocketPath,
		extensionAutoloadPath: extensionAutoloadPath,
	}

	osqueryAutoloadFile, err := os.Create(extensionAutoloadPath)
	if err != nil {
		return nil, fmt.Errorf("creating autoload file: %w", err)
	}
	defer osqueryAutoloadFile.Close()

	return osqueryFilePaths, nil
}

// createOsquerydCommand uses osqueryOptions to return an *exec.Cmd
// which will launch a properly configured osqueryd process.
func (o *OsqueryInstance) createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	args := []string{
		fmt.Sprintf("--logger_plugin=%s", o.opts.loggerPluginFlag),
		fmt.Sprintf("--distributed_plugin=%s", o.opts.distributedPluginFlag),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--host_identifier=uuid",
		"--force=true",
		"--utc",
	}

	if o.knapsack != nil && o.knapsack.WatchdogEnabled() {
		args = append(args, fmt.Sprintf("--watchdog_memory_limit=%d", o.knapsack.WatchdogMemoryLimitMB()))
		args = append(args, fmt.Sprintf("--watchdog_utilization_limit=%d", o.knapsack.WatchdogUtilizationLimitPercent()))
		args = append(args, fmt.Sprintf("--watchdog_delay=%d", o.knapsack.WatchdogDelaySec()))
	} else {
		args = append(args, "--disable_watchdog")
	}

	cmd := exec.Command( //nolint:forbidigo // We trust the autoupdate library to find the correct path
		osquerydBinary,
		args...,
	)

	if o.opts.verbose {
		cmd.Args = append(cmd.Args, "--verbose")
	}

	if o.opts.tlsHostname != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--tls_hostname=%s", o.opts.tlsHostname))
	}

	if o.opts.tlsConfigEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--config_tls_endpoint=%s", o.opts.tlsConfigEndpoint))
	}

	if o.opts.tlsEnrollEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--enroll_tls_endpoint=%s", o.opts.tlsEnrollEndpoint))
	}

	if o.opts.tlsLoggerEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--logger_tls_endpoint=%s", o.opts.tlsLoggerEndpoint))
	}

	if o.opts.tlsDistReadEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--distributed_tls_read_endpoint=%s", o.opts.tlsDistReadEndpoint))
	}

	if o.opts.tlsDistWriteEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--distributed_tls_write_endpoint=%s", o.opts.tlsDistWriteEndpoint))
	}

	if o.opts.tlsServerCerts != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--tls_server_certs=%s", o.opts.tlsServerCerts))
	}

	if o.opts.enrollSecretPath != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--enroll_secret_path=%s", o.opts.enrollSecretPath))
	}

	// Configs aren't expected to change often, so refresh configs
	// every couple minutes. if there's a failure, try again more
	// promptly. Values in seconds. These settings are CLI flags only.
	cmd.Args = append(cmd.Args,
		"--config_refresh=300",
		"--config_accelerated_refresh=30",
	)

	// Augeas. No windows support, and only makes sense if we populated it.
	if paths.augeasPath != "" && runtime.GOOS != "windows" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--augeas_lenses=%s", paths.augeasPath))
	}

	cmd.Args = append(cmd.Args, platformArgs()...)
	if o.opts.stdout != nil {
		cmd.Stdout = o.opts.stdout
	}
	if o.opts.stderr != nil {
		cmd.Stderr = o.opts.stderr
	}

	// Apply user-provided flags last so that they can override other flags set
	// by Launcher (besides the flags below)
	for _, flag := range o.opts.osqueryFlags {
		cmd.Args = append(cmd.Args, "--"+flag)
	}

	// These flags cannot be overridden (to prevent users from breaking Launcher
	// by providing invalid flags)
	cmd.Args = append(
		cmd.Args,
		fmt.Sprintf("--pidfile=%s", paths.pidfilePath),
		fmt.Sprintf("--database_path=%s", paths.databasePath),
		fmt.Sprintf("--extensions_socket=%s", paths.extensionSocketPath),
		fmt.Sprintf("--extensions_autoload=%s", paths.extensionAutoloadPath),
		"--disable_extensions=false",
		"--extensions_timeout=20",
		fmt.Sprintf("--config_plugin=%s", o.opts.configPluginFlag),
		fmt.Sprintf("--extensions_require=%s", strings.Join(o.opts.requiredExtensions(), ",")),
	)

	// On darwin, run osquery using a magic macOS variable to ensure we
	// get proper versions strings back. I'm not totally sure why apple
	// did this, but reading SystemVersion.plist is different when this is set.
	// See:
	// https://eclecticlight.co/2020/08/13/macos-version-numbering-isnt-so-simple/
	// https://github.com/osquery/osquery/pull/6824
	cmd.Env = append(cmd.Env, "SYSTEM_VERSION_COMPAT=0")

	return cmd, nil
}

// StartOsqueryClient will create and return a new osquery client with a connection
// over the socket at the provided path. It will retry for up to 10 seconds to create
// the connection in the event of a failure.
func (o *OsqueryInstance) StartOsqueryClient(paths *osqueryFilePaths) (*osquery.ExtensionManagerClient, error) {
	var client *osquery.ExtensionManagerClient
	if err := backoff.WaitFor(func() error {
		var newErr error
		client, newErr = osquery.NewClient(paths.extensionSocketPath, socketOpenTimeout/2, osquery.DefaultWaitTime(1*time.Second), osquery.MaxWaitTime(maxSocketWaitTime))
		return newErr
	}, socketOpenTimeout, socketOpenInterval); err != nil {
		return nil, fmt.Errorf("could not create an extension client: %w", err)
	}

	return client, nil
}

// startOsqueryExtensionManagerServer takes a set of plugins, creates
// an osquery.NewExtensionManagerServer for them, and then starts it.
func (o *OsqueryInstance) StartOsqueryExtensionManagerServer(name string, socketPath string, client *osquery.ExtensionManagerClient, plugins []osquery.OsqueryPlugin) error {
	o.slogger.Log(context.TODO(), slog.LevelDebug,
		"starting startOsqueryExtensionManagerServer",
		"extension_name", name,
	)

	var extensionManagerServer *osquery.ExtensionManagerServer
	if err := backoff.WaitFor(func() error {
		var newErr error
		extensionManagerServer, newErr = osquery.NewExtensionManagerServer(
			name,
			socketPath,
			osquery.ServerTimeout(1*time.Minute),
			osquery.WithClient(client),
		)
		return newErr
	}, socketOpenTimeout, socketOpenInterval); err != nil {
		o.slogger.Log(context.TODO(), slog.LevelDebug,
			"could not create an extension server",
			"extension_name", name,
			"err", err,
		)
		return fmt.Errorf("could not create an extension server: %w", err)
	}

	extensionManagerServer.RegisterPlugin(plugins...)

	o.emsLock.Lock()
	defer o.emsLock.Unlock()

	o.extensionManagerServers = append(o.extensionManagerServers, extensionManagerServer)

	// Start!
	o.errgroup.Go(func() error {
		defer o.slogger.Log(context.TODO(), slog.LevelDebug,
			"exiting errgroup",
			"errgroup", "run extension manager server",
			"extension_name", name,
		)

		if err := extensionManagerServer.Start(); err != nil {
			o.slogger.Log(context.TODO(), slog.LevelInfo,
				"extension manager server startup got error",
				"err", err,
				"extension_name", name,
			)
			return fmt.Errorf("running extension server: %w", err)
		}
		return errors.New("extension manager server exited")
	})

	// register a shutdown routine
	o.errgroup.Go(func() error {
		defer o.slogger.Log(context.TODO(), slog.LevelDebug,
			"exiting errgroup",
			"errgroup", "shut down extension manager server",
			"extension_name", name,
		)

		<-o.doneCtx.Done()

		o.slogger.Log(context.TODO(), slog.LevelDebug,
			"starting extension shutdown",
			"extension_name", name,
		)

		if err := extensionManagerServer.Shutdown(context.TODO()); err != nil {
			o.slogger.Log(context.TODO(), slog.LevelInfo,
				"got error while shutting down extension server",
				"err", err,
				"extension_name", name,
			)
		}
		return o.doneCtx.Err()
	})

	o.slogger.Log(context.TODO(), slog.LevelDebug,
		"clean finish startOsqueryExtensionManagerServer",
		"extension_name", name,
	)

	return nil
}

func osqueryTempDir() (string, func(), error) {
	tempPath, err := agent.MkdirTemp("")
	if err != nil {
		return "", func() {}, fmt.Errorf("could not make temp path: %w", err)
	}

	return tempPath, func() {
		os.Remove(tempPath)
	}, nil
}

// getOsqueryInfoForLog will log info about an osquery instance. It's
// called when osquery unexpected fails to start. (returns as an
// interface for go-kit's logger)
func getOsqueryInfoForLog(path string) []interface{} {
	msgPairs := []interface{}{
		"path", path,
	}

	file, err := os.Open(path)
	if err != nil {
		return append(msgPairs, "extraerr", fmt.Errorf("opening file: %w", err))
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return append(msgPairs, "extraerr", fmt.Errorf("stat file: %w", err))
	}

	msgPairs = append(
		msgPairs,
		"sizeBytes", fileInfo.Size(),
		"mode", fileInfo.Mode(),
	)

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return append(msgPairs, "extraerr", fmt.Errorf("hashing file: %w", err))
	}

	msgPairs = append(
		msgPairs,
		"sha256", fmt.Sprintf("%x", sum.Sum(nil)),
	)

	return msgPairs
}
