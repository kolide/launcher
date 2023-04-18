package flags

import (
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/launcher"
)

// FlagController is responsible for retrieving flag values from the appropriate sources,
// determining precedence, sanitizing flag values, and notifying observers of changes.
type FlagController struct {
	logger                 log.Logger
	cmdLineOpts            *launcher.Options
	agentFlagsStore        types.KVStore
	overrideMutex          sync.RWMutex
	controlRequestOverride FlagValueOverride
	observers              map[types.FlagsChangeObserver][]keys.FlagKey
}

func NewFlagController(logger log.Logger, agentFlagsStore types.KVStore, opts ...Option) *FlagController {
	fc := &FlagController{
		logger:          logger,
		cmdLineOpts:     &launcher.Options{},
		agentFlagsStore: agentFlagsStore,
		observers:       make(map[types.FlagsChangeObserver][]keys.FlagKey),
	}

	for _, opt := range opts {
		opt(fc)
	}

	return fc
}

// getControlServerValue looks for a control-server-provided value for the key and returns it.
// If a control server value is not found, nil is returned.
func (fc *FlagController) getControlServerValue(key keys.FlagKey) []byte {
	value, err := fc.agentFlagsStore.Get([]byte(key))
	if err != nil {
		level.Debug(fc.logger).Log("msg", "failed to get control server key", "key", key, "err", err)
		return nil
	}

	return value
}

// set stores the value for a FlagKey in the agent flags store. Typically, this is used by the control
// server to provide a control-server-provided value.
func (fc *FlagController) set(key keys.FlagKey, value []byte) error {
	err := fc.agentFlagsStore.Set([]byte(key), value)
	if err != nil {
		level.Debug(fc.logger).Log("msg", "failed to set control server key", "key", key, "err", err)
		return err
	}

	fc.notifyObservers(key)

	return nil
}

// Update bulk replaces agent flags and stores them.
// Observers will be notified of changed flags and deleted flags.
func (fc *FlagController) Update(kvPairs map[string]string) ([]string, error) {
	// Attempt to bulk replace the store with the key-values
	deletedKeys, err := fc.agentFlagsStore.Update(kvPairs)

	// Extract just the keys from the key-value pairs
	updatedKeys := make([]string, len(kvPairs))
	i := 0
	for k := range kvPairs {
		updatedKeys[i] = k
		i++
	}

	// Changed keys is the union of updated keys and deleted keys
	changedKeys := append(updatedKeys, deletedKeys...)

	// Now observers can be notified these keys have possibly changed
	fc.notifyObservers(keys.ToFlagKeys(changedKeys)...)

	return changedKeys, err
}

func (fc *FlagController) RegisterChangeObserver(observer types.FlagsChangeObserver, flagKeys ...keys.FlagKey) {
	fc.observers[observer] = append(fc.observers[observer], flagKeys...)
}

// notifyObservers informs all observers of the keys that they have changed.
func (fc *FlagController) notifyObservers(flagKeys ...keys.FlagKey) {
	for observer, observedKeys := range fc.observers {
		changedKeys := keys.Intersection(observedKeys, flagKeys)

		if len(changedKeys) > 0 {
			observer.FlagsChanged(changedKeys...)
		}
	}
}

func (fc *FlagController) AutoloadedExtensions() []string {
	return fc.cmdLineOpts.AutoloadedExtensions
}

func (fc *FlagController) SetKolideServerURL(url string) error {
	return fc.set(keys.KolideServerURL, []byte(url))
}
func (fc *FlagController) KolideServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.KolideServerURL),
	).get(fc.getControlServerValue(keys.KolideServerURL))
}

func (fc *FlagController) SetKolideHosted(hosted bool) error {
	return fc.set(keys.KolideHosted, boolToBytes(hosted))
}
func (fc *FlagController) KolideHosted() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.KolideHosted))
}

func (fc *FlagController) EnrollSecret() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.EnrollSecret),
	).get(nil)
}

func (fc *FlagController) EnrollSecretPath() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.EnrollSecretPath),
	).get(nil)
}

func (fc *FlagController) RootDirectory() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.RootDirectory),
	).get(nil)
}

func (fc *FlagController) OsquerydPath() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.OsquerydPath),
	).get(nil)
}

func (fc *FlagController) CertPins() [][]byte {
	return fc.cmdLineOpts.CertPins
}

func (fc *FlagController) RootPEM() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.RootPEM),
	).get(nil)
}

func (fc *FlagController) LoggingInterval() time.Duration {
	return NewDurationFlagValue(fc.logger, keys.LoggingInterval,
		WithDefault(fc.cmdLineOpts.LoggingInterval),
		WithMin(5*time.Second),
		WithMax(10*time.Minute),
	).get(fc.getControlServerValue(keys.LoggingInterval))
}

func (fc *FlagController) EnableInitialRunner() bool {
	return NewBoolFlagValue(
		WithDefaultBool(fc.cmdLineOpts.EnableInitialRunner)).
		get(nil)
}

func (fc *FlagController) Transport() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.Transport),
	).get(nil)
}

func (fc *FlagController) LogMaxBytesPerBatch() int {
	return fc.cmdLineOpts.LogMaxBytesPerBatch
}

func (fc *FlagController) SetDesktopEnabled(enabled bool) error {
	return fc.set(keys.DesktopEnabled, boolToBytes(enabled))
}
func (fc *FlagController) DesktopEnabled() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.DesktopEnabled))
}

func (fc *FlagController) SetDesktopUpdateInterval(interval time.Duration) error {
	return fc.set(keys.DesktopUpdateInterval, durationToBytes(interval))
}
func (fc *FlagController) DesktopUpdateInterval() time.Duration {
	return NewDurationFlagValue(fc.logger, keys.DesktopUpdateInterval,
		WithDefault(5*time.Second),
		WithMin(5*time.Second),
		WithMax(10*time.Minute),
	).get(fc.getControlServerValue(keys.DesktopUpdateInterval))
}

func (fc *FlagController) SetDesktopMenuRefreshInterval(interval time.Duration) error {
	return fc.set(keys.DesktopMenuRefreshInterval, durationToBytes(interval))
}
func (fc *FlagController) DesktopMenuRefreshInterval() time.Duration {
	return NewDurationFlagValue(fc.logger, keys.DesktopMenuRefreshInterval,
		WithDefault(15*time.Minute),
		WithMin(5*time.Minute),
		WithMax(60*time.Minute),
	).get(fc.getControlServerValue(keys.DesktopMenuRefreshInterval))
}

func (fc *FlagController) SetDebugServerData(debug bool) error {
	return fc.set(keys.DebugServerData, boolToBytes(debug))
}
func (fc *FlagController) DebugServerData() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.DebugServerData))
}

func (fc *FlagController) SetForceControlSubsystems(force bool) error {
	return fc.set(keys.ForceControlSubsystems, boolToBytes(force))
}
func (fc *FlagController) ForceControlSubsystems() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.ForceControlSubsystems))
}

func (fc *FlagController) SetControlServerURL(url string) error {
	return fc.set(keys.ControlServerURL, []byte(url))
}
func (fc *FlagController) ControlServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.ControlServerURL),
	).get(fc.getControlServerValue(keys.ControlServerURL))
}

func (fc *FlagController) SetControlRequestInterval(interval time.Duration) error {
	return fc.set(keys.ControlRequestInterval, durationToBytes(interval))
}
func (fc *FlagController) SetControlRequestIntervalOverride(interval, duration time.Duration) {
	// Always notify observers when overrides start, so they know to refresh.
	// Defering this before defering unlocking the mutex so that notifications occur outside of the critical section.
	defer fc.notifyObservers(keys.ControlRequestInterval)

	fc.overrideMutex.Lock()
	defer fc.overrideMutex.Unlock()

	if fc.controlRequestOverride == nil || fc.controlRequestOverride.Value() == nil {
		// Creating the override implicitly causes future ControlRequestInterval retrievals to use the override until expiration
		fc.controlRequestOverride = &Override{}
	}

	overrideExpired := func(key keys.FlagKey) {
		// Always notify observers when overrides expire, so they know to refresh.
		// Defering this before defering unlocking the mutex so that notifications occur outside of the critical section.
		defer fc.notifyObservers(key)

		fc.overrideMutex.Lock()
		defer fc.overrideMutex.Unlock()

		// Deleting the override implictly allows the next value to take precedence
		fc.controlRequestOverride = nil
	}

	// Start a new override, or re-start an existing one with a new value, duration, and expiration
	fc.controlRequestOverride.Start(keys.ControlRequestInterval, interval, duration, overrideExpired)
}
func (fc *FlagController) ControlRequestInterval() time.Duration {
	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	return NewDurationFlagValue(fc.logger, keys.ControlRequestInterval,
		WithOverride(fc.controlRequestOverride),
		WithDefault(fc.cmdLineOpts.ControlRequestInterval),
		WithMin(5*time.Second),
		WithMax(10*time.Minute),
	).get(fc.getControlServerValue(keys.ControlRequestInterval))
}

func (fc *FlagController) SetDisableControlTLS(disabled bool) error {
	return fc.set(keys.DisableControlTLS, boolToBytes(disabled))
}
func (fc *FlagController) DisableControlTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.DisableControlTLS)).get(fc.getControlServerValue(keys.DisableControlTLS))
}

func (fc *FlagController) SetInsecureControlTLS(disabled bool) error {
	return fc.set(keys.InsecureControlTLS, boolToBytes(disabled))
}
func (fc *FlagController) InsecureControlTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.InsecureControlTLS)).get(fc.getControlServerValue(keys.InsecureControlTLS))
}

func (fc *FlagController) SetInsecureTLS(insecure bool) error {
	return fc.set(keys.InsecureTLS, boolToBytes(insecure))
}
func (fc *FlagController) InsecureTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.InsecureTLS)).get(fc.getControlServerValue(keys.InsecureTLS))
}

func (fc *FlagController) SetInsecureTransportTLS(insecure bool) error {
	return fc.set(keys.InsecureTransportTLS, boolToBytes(insecure))
}
func (fc *FlagController) InsecureTransportTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.InsecureTransport)).get(fc.getControlServerValue(keys.InsecureTransportTLS))
}

func (fc *FlagController) SetIAmBreakingEELicense(disabled bool) error {
	return fc.set(keys.IAmBreakingEELicense, boolToBytes(disabled))
}
func (fc *FlagController) IAmBreakingEELicense() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.IAmBreakingEELicense)).get(fc.getControlServerValue(keys.IAmBreakingEELicense))
}

func (fc *FlagController) SetDebug(debug bool) error {
	return fc.set(keys.Debug, boolToBytes(debug))
}
func (fc *FlagController) Debug() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.Debug)).get(fc.getControlServerValue(keys.Debug))
}

func (fc *FlagController) SetDebugLogFile(file string) error {
	return fc.set(keys.DebugLogFile, []byte(file))
}
func (fc *FlagController) DebugLogFile() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.DebugLogFile),
	).get(fc.getControlServerValue(keys.DebugLogFile))
}

func (fc *FlagController) SetOsqueryVerbose(verbose bool) error {
	return fc.set(keys.OsqueryVerbose, boolToBytes(verbose))
}
func (fc *FlagController) OsqueryVerbose() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.OsqueryVerbose)).get(fc.getControlServerValue(keys.OsqueryVerbose))
}

func (fc *FlagController) OsqueryFlags() []string {
	return fc.cmdLineOpts.OsqueryFlags
}

func (fc *FlagController) OsqueryTlsConfigEndpoint() string {
	return fc.cmdLineOpts.OsqueryTlsConfigEndpoint
}
func (fc *FlagController) OsqueryTlsEnrollEndpoint() string {
	return fc.cmdLineOpts.OsqueryTlsEnrollEndpoint
}
func (fc *FlagController) OsqueryTlsLoggerEndpoint() string {
	return fc.cmdLineOpts.OsqueryTlsLoggerEndpoint
}
func (fc *FlagController) OsqueryTlsDistributedReadEndpoint() string {
	return fc.cmdLineOpts.OsqueryTlsDistributedReadEndpoint
}
func (fc *FlagController) OsqueryTlsDistributedWriteEndpoint() string {
	return fc.cmdLineOpts.OsqueryTlsDistributedWriteEndpoint
}

func (fc *FlagController) SetAutoupdate(enabled bool) error {
	return fc.set(keys.Autoupdate, boolToBytes(enabled))
}
func (fc *FlagController) Autoupdate() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.Autoupdate)).get(fc.getControlServerValue(keys.Autoupdate))
}

func (fc *FlagController) SetNotaryServerURL(url string) error {
	return fc.set(keys.NotaryServerURL, []byte(url))
}
func (fc *FlagController) NotaryServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.NotaryServerURL),
	).get(fc.getControlServerValue(keys.NotaryServerURL))
}

func (fc *FlagController) SetTufServerURL(url string) error {
	return fc.set(keys.TufServerURL, []byte(url))
}
func (fc *FlagController) TufServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.TufServerURL),
	).get(fc.getControlServerValue(keys.TufServerURL))
}

func (fc *FlagController) SetMirrorServerURL(url string) error {
	return fc.set(keys.MirrorServerURL, []byte(url))
}
func (fc *FlagController) MirrorServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.MirrorServerURL),
	).get(fc.getControlServerValue(keys.MirrorServerURL))
}

func (fc *FlagController) SetAutoupdateInterval(interval time.Duration) error {
	return fc.set(keys.AutoupdateInterval, durationToBytes(interval))
}
func (fc *FlagController) AutoupdateInterval() time.Duration {
	return NewDurationFlagValue(fc.logger, keys.AutoupdateInterval,
		WithDefault(fc.cmdLineOpts.AutoupdateInterval),
		WithMin(1*time.Minute),
		WithMax(24*time.Hour),
	).get(fc.getControlServerValue(keys.AutoupdateInterval))
}

func (fc *FlagController) SetUpdateChannel(channel string) error {
	return fc.set(keys.UpdateChannel, []byte(channel))
}
func (fc *FlagController) UpdateChannel() string {
	return NewStringFlagValue(
		WithSanitizer(autoupdate.SanitizeUpdateChannel),
		WithDefaultString(string(fc.cmdLineOpts.UpdateChannel)),
	).get(fc.getControlServerValue(keys.UpdateChannel))
}

func (fc *FlagController) SetNotaryPrefix(prefix string) error {
	return fc.set(keys.NotaryPrefix, []byte(prefix))
}
func (fc *FlagController) NotaryPrefix() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.NotaryPrefix),
	).get(fc.getControlServerValue(keys.NotaryPrefix))
}

func (fc *FlagController) SetAutoupdateInitialDelay(delay time.Duration) error {
	return fc.set(keys.AutoupdateInitialDelay, durationToBytes(delay))
}
func (fc *FlagController) AutoupdateInitialDelay() time.Duration {
	return NewDurationFlagValue(fc.logger, keys.AutoupdateInitialDelay,
		WithDefault(fc.cmdLineOpts.AutoupdateInitialDelay),
		WithMin(5*time.Second),
		WithMax(12*time.Hour),
	).get(fc.getControlServerValue(keys.AutoupdateInitialDelay))
}

func (fc *FlagController) SetUpdateDirectory(directory string) error {
	return fc.set(keys.UpdateDirectory, []byte(directory))
}
func (fc *FlagController) UpdateDirectory() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.UpdateDirectory),
	).get(fc.getControlServerValue(keys.UpdateDirectory))
}
