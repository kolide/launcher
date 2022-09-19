package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/osquery/osquery-go"
	"github.com/pkg/errors"
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

// WithOsquerydBinary is a functional option which allows the user to define the
// path of the osqueryd binary which will be launched. This should only be called
// once as only one binary will be executed. Defining the path to the osqueryd
// binary is optional. If it is not explicitly defined by the caller, an osqueryd
// binary will be looked for in the current $PATH.
func WithOsquerydBinary(path string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.binaryPath = path
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

// WithLogger is a functional option which allows the user to pass a log.Logger
// to be used for logging osquery instance status.
func WithLogger(logger log.Logger) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.logger = logger
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

// WithAutoloadedExtensions defines a list of extensions to load
// via the osquery autoloading.
func WithAutoloadedExtensions(extensions ...string) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.opts.autoloadedExtensions = append(i.opts.autoloadedExtensions, extensions...)
	}
}

// OsqueryInstance is the type which represents a currently running instance
// of osqueryd.
type OsqueryInstance struct {
	opts   osqueryOptions
	logger log.Logger
	// the following are instance artifacts that are created and held as a result
	// of launching an osqueryd process
	errgroup                *errgroup.Group
	doneCtx                 context.Context
	cancel                  context.CancelFunc
	cmd                     *exec.Cmd
	emsLock                 sync.RWMutex
	extensionManagerServers []*osquery.ExtensionManagerServer
	extensionManagerClient  *osquery.ExtensionManagerClient
	clientLock              sync.Mutex
	paths                   *osqueryFilePaths
	rmRootDirectory         func()
	usingTempDir            bool
	stats                   *history.Instance
}

// Healthy will check to determine whether or not the osquery process that is
// being managed by the current instantiation of this OsqueryInstance is
// healthy. If the instance is healthy, it returns nil.
func (o *OsqueryInstance) Healthy() error {
	o.emsLock.RLock()
	defer o.emsLock.RUnlock()

	if len(o.extensionManagerServers) == 0 || o.extensionManagerClient == nil {
		return errors.New("instance not started")
	}

	for _, srv := range o.extensionManagerServers {
		serverStatus, err := srv.Ping(context.TODO())
		if err != nil {
			return errors.Wrap(err, "could not ping extension server")
		}
		if serverStatus.Code != 0 {
			return errors.Errorf("ping extension server returned %d: %s",
				serverStatus.Code,
				serverStatus.Message,
			)
		}
	}

	o.clientLock.Lock()
	defer o.clientLock.Unlock()

	clientStatus, err := o.extensionManagerClient.Ping()
	if err != nil {
		return errors.Wrap(err, "could not ping osquery extension client")
	}
	if clientStatus.Code != 0 {
		return errors.Errorf("ping extension client returned %d: %s",
			clientStatus.Code,
			clientStatus.Message,
		)
	}

	return nil
}

func (o *OsqueryInstance) Query(query string) ([]map[string]string, error) {
	o.clientLock.Lock()
	defer o.clientLock.Unlock()

	if o.extensionManagerClient == nil {
		return nil, errors.New("client not ready")
	}

	resp, err := o.extensionManagerClient.Query(query)
	if err != nil {
		return nil, errors.Wrap(err, "could not query the extension manager client")
	}
	if resp.Status.Code != int32(0) {
		return nil, errors.New(resp.Status.Message)
	}

	return resp.Response, nil
}

type osqueryOptions struct {
	// the following are options which may or may not be set by the functional
	// options included by the caller of LaunchOsqueryInstance
	augeasLensFunc        func(dir string) error
	binaryPath            string
	configPluginFlag      string
	distributedPluginFlag string
	extensionPlugins      []osquery.OsqueryPlugin
	autoloadedExtensions  []string
	extensionSocketPath   string
	enrollSecretPath      string
	loggerPluginFlag      string
	osqueryFlags          []string
	retries               uint
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
	verbose               bool
}

// requiredExtensions returns a unique list of external
// extensions. These are extensions we expect osquery to pause start
// for.
func (o osqueryOptions) requiredExtensions() []string {
	extensionsMap := make(map[string]bool)
	requiredExtensions := make([]string, 0)

	for _, extension := range append([]string{o.loggerPluginFlag, o.configPluginFlag, o.distributedPluginFlag}, o.autoloadedExtensions...) {
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

	i.logger = log.NewNopLogger()

	return i
}

// osqueryFilePaths is a struct which contains the relevant file paths needed to
// launch an osqueryd instance.
type osqueryFilePaths struct {
	augeasPath            string
	databasePath          string
	extensionAutoloadPath string
	extensionSocketPath   string
	extensionPaths        []string
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

	osqueryFilePaths := &osqueryFilePaths{
		pidfilePath:           filepath.Join(opts.rootDirectory, "osquery.pid"),
		databasePath:          filepath.Join(opts.rootDirectory, "osquery.db"),
		augeasPath:            filepath.Join(opts.rootDirectory, "augeas-lenses"),
		extensionSocketPath:   extensionSocketPath,
		extensionAutoloadPath: extensionAutoloadPath,
		extensionPaths:        make([]string, len(opts.autoloadedExtensions)),
	}

	osqueryAutoloadFile, err := os.Create(extensionAutoloadPath)
	if err != nil {
		return nil, errors.Wrap(err, "creating autoload file")
	}
	defer osqueryAutoloadFile.Close()

	if len(opts.autoloadedExtensions) == 0 {
		return osqueryFilePaths, nil
	}

	// Determine the path to the extension
	exPath, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "finding path of launcher executable")
	}

	for index, extension := range opts.autoloadedExtensions {
		// first see if we just got a file name and check to see if it exists in the executable directory
		extensionPath := filepath.Join(autoupdate.FindBaseDir(exPath), extension)

		if _, err := os.Stat(extensionPath); err != nil {
			// if we got an error, try the raw flag
			extensionPath = extension

			if _, err := os.Stat(extensionPath); err != nil {
				if os.IsNotExist(err) {
					return nil, errors.Wrapf(err, "extension path does not exist: %s", extension)
				} else {
					return nil, errors.Wrapf(err, "could not stat extension path")
				}
			}
		}

		osqueryFilePaths.extensionPaths[index] = extensionPath

		_, err := osqueryAutoloadFile.WriteString(fmt.Sprintf("%s\n", extensionPath))
		if err != nil {
			return nil, errors.Wrapf(err, "writing to autoload file")
		}
	}

	return osqueryFilePaths, nil
}

// createOsquerydCommand uses osqueryOptions to return an *exec.Cmd
// which will launch a properly configured osqueryd process.
func (opts *osqueryOptions) createOsquerydCommand(osquerydBinary string, paths *osqueryFilePaths) (*exec.Cmd, error) {
	// Create the reference instance for the running osquery instance
	cmd := exec.Command(
		osquerydBinary,
		fmt.Sprintf("--logger_plugin=%s", opts.loggerPluginFlag),
		fmt.Sprintf("--distributed_plugin=%s", opts.distributedPluginFlag),
		"--disable_distributed=false",
		"--distributed_interval=5",
		"--pack_delimiter=:",
		"--host_identifier=uuid",
		"--force=true",
		"--disable_watchdog",
		"--utc",
	)

	if opts.verbose {
		cmd.Args = append(cmd.Args, "--verbose")
	}

	if opts.tlsHostname != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--tls_hostname=%s", opts.tlsHostname))
	}

	if opts.tlsConfigEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--config_tls_endpoint=%s", opts.tlsConfigEndpoint))

	}

	if opts.tlsEnrollEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--enroll_tls_endpoint=%s", opts.tlsEnrollEndpoint))
	}

	if opts.tlsLoggerEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--logger_tls_endpoint=%s", opts.tlsLoggerEndpoint))
	}

	if opts.tlsDistReadEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--distributed_tls_read_endpoint=%s", opts.tlsDistReadEndpoint))
	}

	if opts.tlsDistWriteEndpoint != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--distributed_tls_write_endpoint=%s", opts.tlsDistWriteEndpoint))
	}

	if opts.tlsServerCerts != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--tls_server_certs=%s", opts.tlsServerCerts))
	}

	if opts.enrollSecretPath != "" {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--enroll_secret_path=%s", opts.enrollSecretPath))
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
	if opts.stdout != nil {
		cmd.Stdout = opts.stdout
	}
	if opts.stderr != nil {
		cmd.Stderr = opts.stderr
	}

	// Apply user-provided flags last so that they can override other flags set
	// by Launcher (besides the flags below)
	for _, flag := range opts.osqueryFlags {
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
		fmt.Sprintf("--config_plugin=%s", opts.configPluginFlag),
		fmt.Sprintf("--extensions_require=%s", strings.Join(opts.requiredExtensions(), ",")),
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

// startOsqueryExtensionManagerServer takes a set of plugins, creates
// an osquery.NewExtensionManagerServer for them, and then starts it.
func (o *OsqueryInstance) StartOsqueryExtensionManagerServer(name string, socketPath string, plugins []osquery.OsqueryPlugin) error {
	logger := log.With(o.logger, "extensionMangerServer", name)

	level.Debug(logger).Log("msg", "Starting startOsqueryExtensionManagerServer")

	var extensionManagerServer *osquery.ExtensionManagerServer
	if err := backoff.WaitFor(func() error {
		var newErr error
		extensionManagerServer, newErr = osquery.NewExtensionManagerServer(
			name,
			socketPath,
			osquery.ServerTimeout(1*time.Minute),
		)
		return newErr
	}, socketOpenTimeout, socketOpenInterval); err != nil {
		level.Debug(logger).Log("msg", "could not create an extension server", "err", err)
		return errors.Wrap(err, "could not create an extension server")
	}

	extensionManagerServer.RegisterPlugin(plugins...)

	o.emsLock.Lock()
	defer o.emsLock.Unlock()

	o.extensionManagerServers = append(o.extensionManagerServers, extensionManagerServer)

	// Start!
	o.errgroup.Go(func() error {
		if err := extensionManagerServer.Start(); err != nil {
			level.Info(logger).Log("msg", "Extension manager server startup got error", "err", err)
			return errors.Wrap(err, "running extension server")
		}
		return errors.New("extension manager server exited")
	})

	// register a shutdown routine
	o.errgroup.Go(func() error {
		<-o.doneCtx.Done()
		level.Debug(logger).Log("msg", "Starting extension shutdown")
		if err := extensionManagerServer.Shutdown(context.TODO()); err != nil {
			level.Info(o.logger).Log(
				"msg", "Got error while shutting down extension server",
				"err", err,
			)
		}
		return o.doneCtx.Err()
	})

	level.Debug(logger).Log("msg", "Clean finish startOsqueryExtensionManagerServer")

	return nil
}

func osqueryTempDir() (string, func(), error) {
	tempPath, err := ioutil.TempDir("", "")
	if err != nil {
		return "", func() {}, errors.Wrap(err, "could not make temp path")
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
		return append(msgPairs, "extraerr", errors.Wrap(err, "opening file"))
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return append(msgPairs, "extraerr", errors.Wrap(err, "stat file"))
	}

	msgPairs = append(
		msgPairs,
		"sizeBytes", fileInfo.Size(),
		"mode", fileInfo.Mode(),
	)

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return append(msgPairs, "extraerr", errors.Wrap(err, "hashing file"))
	}

	msgPairs = append(
		msgPairs,
		"sha256", fmt.Sprintf("%x", sum.Sum(nil)),
	)

	return msgPairs
}
