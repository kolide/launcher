package flags

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/launcher"
	"golang.org/x/exp/maps"
)

// FlagController is responsible for retrieving flag values from the appropriate sources,
// determining precedence, sanitizing flag values, and notifying observers of changes.
type FlagController struct {
	slogger         *slog.Logger
	cmdLineOpts     *launcher.Options
	agentFlagsStore types.KVStore
	overrideMutex   sync.RWMutex
	overrides       map[keys.FlagKey]*Override
	observers       map[types.FlagsChangeObserver][]keys.FlagKey
	observersMutex  sync.RWMutex
}

func NewFlagController(slogger *slog.Logger, agentFlagsStore types.KVStore, opts ...Option) *FlagController {
	fc := &FlagController{
		slogger:         slogger.With("component", "flag_controller"),
		cmdLineOpts:     &launcher.Options{},
		agentFlagsStore: agentFlagsStore,
		observers:       make(map[types.FlagsChangeObserver][]keys.FlagKey),
		overrides:       make(map[keys.FlagKey]*Override),
	}

	for _, opt := range opts {
		opt(fc)
	}

	return fc
}

// getControlServerValue looks for a control-server-provided value for the key and returns it.
// If a control server value is not found, nil is returned.
func (fc *FlagController) getControlServerValue(key keys.FlagKey) []byte {
	if fc == nil || fc.agentFlagsStore == nil {
		return nil
	}

	value, err := fc.agentFlagsStore.Get([]byte(key))
	if err != nil {
		fc.slogger.Log(context.TODO(), slog.LevelDebug,
			"failed to get control server key",
			"key", key,
			"err", err,
		)
		return nil
	}

	return value
}

// setControlServerValue stores a control-server-provided value in the agent flags store.
func (fc *FlagController) setControlServerValue(key keys.FlagKey, value []byte) error {
	ctx, span := observability.StartSpan(context.TODO(), "key", key.String())
	defer span.End()

	if fc == nil || fc.agentFlagsStore == nil {
		return errors.New("agentFlagsStore is nil")
	}

	err := fc.agentFlagsStore.Set([]byte(key), value)
	if err != nil {
		fc.slogger.Log(ctx, slog.LevelDebug,
			"failed to set control server key",
			"key", key,
			"err", err,
		)
		return err
	}

	fc.notifyObservers(ctx, key)

	return nil
}

// Update bulk replaces agent flags and stores them.
// Observers will be notified of changed flags and deleted flags.
func (fc *FlagController) Update(kvPairs map[string]string) ([]string, error) {
	ctx, span := observability.StartSpan(context.Background())
	defer span.End()

	// Attempt to bulk replace the store with the key-values
	deletedKeys, err := fc.agentFlagsStore.Update(kvPairs)

	// Extract just the keys from the key-value pairs
	updatedKeys := maps.Keys(kvPairs)

	// Changed keys is the union of updated keys and deleted keys
	changedKeys := append(updatedKeys, deletedKeys...)

	// Now observers can be notified these keys have possibly changed
	fc.notifyObservers(ctx, keys.ToFlagKeys(changedKeys)...)

	return changedKeys, err
}

func (fc *FlagController) RegisterChangeObserver(observer types.FlagsChangeObserver, flagKeys ...keys.FlagKey) {
	fc.observersMutex.Lock()
	defer fc.observersMutex.Unlock()

	fc.observers[observer] = append(fc.observers[observer], flagKeys...)
}

func (fc *FlagController) DeregisterChangeObserver(observer types.FlagsChangeObserver) {
	fc.observersMutex.Lock()
	defer fc.observersMutex.Unlock()

	if _, ok := fc.observers[observer]; !ok {
		// Nothing to do
		return
	}

	delete(fc.observers, observer)
}

// notifyObservers informs all observers of the keys that they have changed.
func (fc *FlagController) notifyObservers(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	fc.observersMutex.RLock()
	span.AddEvent("observers_lock_acquired")
	defer func() {
		fc.observersMutex.RUnlock()
		span.AddEvent("observers_lock_released")
	}()

	for observer, observedKeys := range fc.observers {
		changedKeys := keys.Intersection(observedKeys, flagKeys)

		if len(changedKeys) > 0 {
			observer.FlagsChanged(ctx, changedKeys...)
		}
	}
}

func (fc *FlagController) overrideFlag(ctx context.Context, key keys.FlagKey, duration time.Duration, value any) {
	ctx, span := observability.StartSpan(ctx, "key", key.String())
	defer span.End()

	// Always notify observers when overrides start, so they know to refresh.
	// Defering this before defering unlocking the mutex so that notifications occur outside of the critical section.
	defer fc.notifyObservers(ctx, key)

	fc.overrideMutex.Lock()
	span.AddEvent("override_lock_acquired")

	defer func() {
		fc.overrideMutex.Unlock()
		span.AddEvent("override_lock_released")
	}()

	fc.slogger.Log(ctx, slog.LevelInfo,
		"overriding flag",
		"key", key,
		"value", value,
		"duration", duration,
	)

	override, ok := fc.overrides[key]
	if !ok || override.Value() == nil {
		// Creating the override implicitly causes future flag value retrievals to use the override until expiration
		override = &Override{}
		fc.overrides[key] = override
	}

	overrideExpired := func(key keys.FlagKey) {
		ctx, span := observability.StartSpan(context.TODO(), "key", key.String())
		defer span.End()

		// Always notify observers when overrides expire, so they know to refresh.
		// Defering this before defering unlocking the mutex so that notifications occur outside of the critical section.
		defer fc.notifyObservers(ctx, key)

		fc.overrideMutex.Lock()
		span.AddEvent("override_lock_acquired")

		defer func() {
			fc.overrideMutex.Unlock()
			span.AddEvent("override_lock_released")
		}()

		// Deleting the override implictly allows the next value to take precedence
		delete(fc.overrides, key)
	}

	// Start a new override, or re-start an existing one with a new value, duration, and expiration
	fc.overrides[key].Start(key, value, duration, overrideExpired)
}

func (fc *FlagController) SetKolideServerURL(url string) error {
	return fc.setControlServerValue(keys.KolideServerURL, []byte(url))
}
func (fc *FlagController) KolideServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.KolideServerURL),
	).get(fc.getControlServerValue(keys.KolideServerURL))
}

func (fc *FlagController) SetKolideHosted(hosted bool) error {
	return fc.setControlServerValue(keys.KolideHosted, boolToBytes(hosted))
}
func (fc *FlagController) KolideHosted() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.KolideHosted)).get(fc.getControlServerValue(keys.KolideHosted))
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

func (fc *FlagController) SetLoggingInterval(interval time.Duration) error {
	return fc.setControlServerValue(keys.LoggingInterval, durationToBytes(interval))
}
func (fc *FlagController) LoggingInterval() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.LoggingInterval,
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
	return fc.setControlServerValue(keys.DesktopEnabled, boolToBytes(enabled))
}
func (fc *FlagController) DesktopEnabled() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.DesktopEnabled))
}

func (fc *FlagController) SetDesktopUpdateInterval(interval time.Duration) error {
	return fc.setControlServerValue(keys.DesktopUpdateInterval, durationToBytes(interval))
}
func (fc *FlagController) DesktopUpdateInterval() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.DesktopUpdateInterval,
		WithDefault(5*time.Second),
		WithMin(5*time.Second),
		WithMax(10*time.Minute),
	).get(fc.getControlServerValue(keys.DesktopUpdateInterval))
}

func (fc *FlagController) SetDesktopMenuRefreshInterval(interval time.Duration) error {
	return fc.setControlServerValue(keys.DesktopMenuRefreshInterval, durationToBytes(interval))
}
func (fc *FlagController) DesktopMenuRefreshInterval() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.DesktopMenuRefreshInterval,
		WithDefault(15*time.Minute),
		WithMin(5*time.Minute),
		WithMax(60*time.Minute),
	).get(fc.getControlServerValue(keys.DesktopMenuRefreshInterval))
}

func (fc *FlagController) SetDebugServerData(debug bool) error {
	return fc.setControlServerValue(keys.DebugServerData, boolToBytes(debug))
}
func (fc *FlagController) DebugServerData() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.DebugServerData))
}

func (fc *FlagController) SetForceControlSubsystems(force bool) error {
	return fc.setControlServerValue(keys.ForceControlSubsystems, boolToBytes(force))
}
func (fc *FlagController) ForceControlSubsystems() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.ForceControlSubsystems))
}

func (fc *FlagController) SetControlServerURL(url string) error {
	return fc.setControlServerValue(keys.ControlServerURL, []byte(url))
}
func (fc *FlagController) ControlServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.ControlServerURL),
	).get(fc.getControlServerValue(keys.ControlServerURL))
}

func (fc *FlagController) SetControlRequestInterval(interval time.Duration) error {
	return fc.setControlServerValue(keys.ControlRequestInterval, durationToBytes(interval))
}
func (fc *FlagController) SetControlRequestIntervalOverride(value time.Duration, duration time.Duration) {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	fc.overrideFlag(ctx, keys.ControlRequestInterval, duration, value)
}
func (fc *FlagController) ControlRequestInterval() time.Duration {
	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	return NewDurationFlagValue(fc.slogger, keys.ControlRequestInterval,
		WithOverride(fc.overrides[keys.ControlRequestInterval]),
		WithDefault(fc.cmdLineOpts.ControlRequestInterval),
		WithMin(5*time.Second),
		WithMax(10*time.Minute),
	).get(fc.getControlServerValue(keys.ControlRequestInterval))
}

func (fc *FlagController) SetAllowOverlyBroadDt4aAcceleration(enabled bool) error {
	return fc.setControlServerValue(keys.AllowOverlyBroadDt4aAcceleration, boolToBytes(enabled))
}
func (fc *FlagController) AllowOverlyBroadDt4aAcceleration() bool {
	return NewBoolFlagValue(
		WithDefaultBool(false),
	).get(fc.getControlServerValue(keys.AllowOverlyBroadDt4aAcceleration))
}

func (fc *FlagController) SetDisableControlTLS(disabled bool) error {
	return fc.setControlServerValue(keys.DisableControlTLS, boolToBytes(disabled))
}
func (fc *FlagController) DisableControlTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.DisableControlTLS)).get(fc.getControlServerValue(keys.DisableControlTLS))
}

func (fc *FlagController) SetInsecureControlTLS(disabled bool) error {
	return fc.setControlServerValue(keys.InsecureControlTLS, boolToBytes(disabled))
}
func (fc *FlagController) InsecureControlTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.InsecureControlTLS)).get(fc.getControlServerValue(keys.InsecureControlTLS))
}

func (fc *FlagController) SetInsecureTLS(insecure bool) error {
	return fc.setControlServerValue(keys.InsecureTLS, boolToBytes(insecure))
}
func (fc *FlagController) InsecureTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.InsecureTLS)).get(fc.getControlServerValue(keys.InsecureTLS))
}

func (fc *FlagController) SetInsecureTransportTLS(insecure bool) error {
	return fc.setControlServerValue(keys.InsecureTransportTLS, boolToBytes(insecure))
}
func (fc *FlagController) InsecureTransportTLS() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.InsecureTransport)).get(fc.getControlServerValue(keys.InsecureTransportTLS))
}

func (fc *FlagController) IAmBreakingEELicense() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.IAmBreakingEELicense)).get(fc.getControlServerValue(keys.IAmBreakingEELicense))
}

func (fc *FlagController) SetDebug(debug bool) error {
	return fc.setControlServerValue(keys.Debug, boolToBytes(debug))
}
func (fc *FlagController) Debug() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.Debug)).get(fc.getControlServerValue(keys.Debug))
}

func (fc *FlagController) DebugLogFile() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.DebugLogFile),
	).get(fc.getControlServerValue(keys.DebugLogFile))
}

func (fc *FlagController) SetOsqueryVerbose(verbose bool) error {
	return fc.setControlServerValue(keys.OsqueryVerbose, boolToBytes(verbose))
}
func (fc *FlagController) OsqueryVerbose() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.OsqueryVerbose)).get(fc.getControlServerValue(keys.OsqueryVerbose))
}

func (fc *FlagController) SetDistributedForwardingInterval(interval time.Duration) error {
	return fc.setControlServerValue(keys.DistributedForwardingInterval, durationToBytes(interval))
}
func (fc *FlagController) SetDistributedForwardingIntervalOverride(value time.Duration, duration time.Duration) {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	fc.overrideFlag(ctx, keys.DistributedForwardingInterval, duration, value)
}
func (fc *FlagController) DistributedForwardingInterval() time.Duration {
	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	return NewDurationFlagValue(fc.slogger, keys.DistributedForwardingInterval,
		WithOverride(fc.overrides[keys.DistributedForwardingInterval]),
		WithDefault(1*time.Minute),
		WithMin(5*time.Second),
		WithMax(5*time.Minute),
	).get(fc.getControlServerValue(keys.DistributedForwardingInterval))
}

func (fc *FlagController) SetWatchdogEnabled(enable bool) error {
	return fc.setControlServerValue(keys.WatchdogEnabled, boolToBytes(enable))
}
func (fc *FlagController) WatchdogEnabled() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.WatchdogEnabled)).get(fc.getControlServerValue(keys.WatchdogEnabled))
}

func (fc *FlagController) SetWatchdogDelaySec(sec int) error {
	return fc.setControlServerValue(keys.WatchdogDelaySec, intToBytes(sec))
}
func (fc *FlagController) WatchdogDelaySec() int {
	return NewIntFlagValue(fc.slogger, keys.WatchdogDelaySec,
		WithIntValueDefault(fc.cmdLineOpts.WatchdogDelaySec),
		WithIntValueMin(0),
		WithIntValueMax(600),
	).get(fc.getControlServerValue(keys.WatchdogDelaySec))
}

func (fc *FlagController) SetWatchdogMemoryLimitMB(limit int) error {
	return fc.setControlServerValue(keys.WatchdogMemoryLimitMB, intToBytes(limit))
}
func (fc *FlagController) WatchdogMemoryLimitMB() int {
	return NewIntFlagValue(fc.slogger, keys.WatchdogMemoryLimitMB,
		WithIntValueDefault(fc.cmdLineOpts.WatchdogMemoryLimitMB),
		WithIntValueMin(100),
		WithIntValueMax(10000), // 10 GB appears to be the max that osquery will accept
	).get(fc.getControlServerValue(keys.WatchdogMemoryLimitMB))
}

func (fc *FlagController) SetWatchdogUtilizationLimitPercent(limit int) error {
	return fc.setControlServerValue(keys.WatchdogUtilizationLimitPercent, intToBytes(limit))
}
func (fc *FlagController) WatchdogUtilizationLimitPercent() int {
	return NewIntFlagValue(fc.slogger, keys.WatchdogUtilizationLimitPercent,
		WithIntValueDefault(fc.cmdLineOpts.WatchdogUtilizationLimitPercent),
		WithIntValueMin(5),
		WithIntValueMax(100),
	).get(fc.getControlServerValue(keys.WatchdogUtilizationLimitPercent))
}

func (fc *FlagController) OsqueryFlags() []string {
	return fc.cmdLineOpts.OsqueryFlags
}

func (fc *FlagController) CurrentRunningOsqueryVersion() string {
	return NewStringFlagValue(WithDefaultString("")).get(fc.getControlServerValue(keys.CurrentRunningOsqueryVersion))
}

func (fc *FlagController) SetCurrentRunningOsqueryVersion(osqueryversion string) error {
	return fc.setControlServerValue(keys.CurrentRunningOsqueryVersion, []byte(osqueryversion))
}

func (fc *FlagController) SetAutoupdate(enabled bool) error {
	return fc.setControlServerValue(keys.Autoupdate, boolToBytes(enabled))
}
func (fc *FlagController) Autoupdate() bool {
	return NewBoolFlagValue(WithDefaultBool(fc.cmdLineOpts.Autoupdate)).get(fc.getControlServerValue(keys.Autoupdate))
}

func (fc *FlagController) SetTufServerURL(url string) error {
	return fc.setControlServerValue(keys.TufServerURL, []byte(url))
}
func (fc *FlagController) TufServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.TufServerURL),
	).get(fc.getControlServerValue(keys.TufServerURL))
}

func (fc *FlagController) SetMirrorServerURL(url string) error {
	return fc.setControlServerValue(keys.MirrorServerURL, []byte(url))
}
func (fc *FlagController) MirrorServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.MirrorServerURL),
	).get(fc.getControlServerValue(keys.MirrorServerURL))
}

func (fc *FlagController) SetAutoupdateInterval(interval time.Duration) error {
	return fc.setControlServerValue(keys.AutoupdateInterval, durationToBytes(interval))
}
func (fc *FlagController) AutoupdateInterval() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.AutoupdateInterval,
		WithDefault(fc.cmdLineOpts.AutoupdateInterval),
		WithMin(1*time.Minute),
		WithMax(24*time.Hour),
	).get(fc.getControlServerValue(keys.AutoupdateInterval))
}

func (fc *FlagController) SetUpdateChannel(channel string) error {
	return fc.setControlServerValue(keys.UpdateChannel, []byte(channel))
}
func (fc *FlagController) UpdateChannel() string {
	return NewStringFlagValue(
		WithSanitizer(launcher.SanitizeUpdateChannel),
		WithDefaultString(string(fc.cmdLineOpts.UpdateChannel)),
	).get(fc.getControlServerValue(keys.UpdateChannel))
}

func (fc *FlagController) SetAutoupdateInitialDelay(delay time.Duration) error {
	return fc.setControlServerValue(keys.AutoupdateInitialDelay, durationToBytes(delay))
}
func (fc *FlagController) AutoupdateInitialDelay() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.AutoupdateInitialDelay,
		WithDefault(fc.cmdLineOpts.AutoupdateInitialDelay),
		WithMin(5*time.Second),
		WithMax(12*time.Hour),
	).get(fc.getControlServerValue(keys.AutoupdateInitialDelay))
}

func (fc *FlagController) SetUpdateDirectory(directory string) error {
	return fc.setControlServerValue(keys.UpdateDirectory, []byte(directory))
}
func (fc *FlagController) UpdateDirectory() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.UpdateDirectory),
	).get(fc.getControlServerValue(keys.UpdateDirectory))
}

func (fc *FlagController) SetPinnedLauncherVersion(version string) error {
	return fc.setControlServerValue(keys.PinnedLauncherVersion, []byte(version))
}
func (fc *FlagController) PinnedLauncherVersion() string {
	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	return NewStringFlagValue(
		WithOverrideString(fc.overrides[keys.PinnedLauncherVersion]),
		WithDefaultString(""),
		WithSanitizer(func(version string) string {
			return tuf.SanitizePinnedVersion("launcher", version)
		}),
	).get(fc.getControlServerValue(keys.PinnedLauncherVersion))
}

func (fc *FlagController) SetPinnedOsquerydVersion(version string) error {
	return fc.setControlServerValue(keys.PinnedOsquerydVersion, []byte(version))
}
func (fc *FlagController) PinnedOsquerydVersion() string {
	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	return NewStringFlagValue(
		WithOverrideString(fc.overrides[keys.PinnedOsquerydVersion]),
		WithDefaultString(""),
		WithSanitizer(func(version string) string {
			return tuf.SanitizePinnedVersion("osqueryd", version)
		}),
	).get(fc.getControlServerValue(keys.PinnedOsquerydVersion))
}

func (fc *FlagController) SetExportTraces(enabled bool) error {
	return fc.setControlServerValue(keys.ExportTraces, boolToBytes(enabled))
}
func (fc *FlagController) SetExportTracesOverride(value bool, duration time.Duration) {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	fc.overrideFlag(ctx, keys.ExportTraces, duration, value)
}
func (fc *FlagController) ExportTraces() bool {
	return NewBoolFlagValue(
		WithBoolOverride(fc.overrides[keys.ExportTraces]),
		WithDefaultBool(fc.cmdLineOpts.ExportTraces),
	).get(fc.getControlServerValue(keys.ExportTraces))
}

func (fc *FlagController) SetLauncherWatchdogEnabled(enabled bool) error {
	return fc.setControlServerValue(keys.LauncherWatchdogEnabled, boolToBytes(enabled))
}

func (fc *FlagController) LauncherWatchdogEnabled() bool {
	return NewBoolFlagValue(
		WithDefaultBool(false),
	).get(fc.getControlServerValue(keys.LauncherWatchdogEnabled))
}

func (fc *FlagController) SetSystrayRestartEnabled(enabled bool) error {
	return fc.setControlServerValue(keys.SystrayRestartEnabled, boolToBytes(enabled))
}

func (fc *FlagController) SystrayRestartEnabled() bool {
	return NewBoolFlagValue(
		WithDefaultBool(false),
	).get(fc.getControlServerValue(keys.SystrayRestartEnabled))
}

func (fc *FlagController) SetTraceSamplingRate(rate float64) error {
	return fc.setControlServerValue(keys.TraceSamplingRate, float64ToBytes(rate))
}
func (fc *FlagController) SetTraceSamplingRateOverride(value float64, duration time.Duration) {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	fc.overrideFlag(ctx, keys.TraceSamplingRate, duration, value)
}
func (fc *FlagController) TraceSamplingRate() float64 {
	return NewFloat64FlagValue(fc.slogger, keys.LoggingInterval,
		WithFloat64ValueOverride(fc.overrides[keys.TraceSamplingRate]),
		WithFloat64ValueDefault(fc.cmdLineOpts.TraceSamplingRate),
		WithFloat64ValueMin(0.0),
		WithFloat64ValueMax(1.0),
	).get(fc.getControlServerValue(keys.TraceSamplingRate))
}

func (fc *FlagController) SetTraceBatchTimeout(duration time.Duration) error {
	return fc.setControlServerValue(keys.TraceBatchTimeout, durationToBytes(duration))
}
func (fc *FlagController) TraceBatchTimeout() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.TraceBatchTimeout,
		WithDefault(1*time.Minute),
		WithMin(5*time.Second),
		WithMax(1*time.Hour),
	).get(fc.getControlServerValue(keys.TraceBatchTimeout))
}

func (fc *FlagController) SetLogIngestServerURL(url string) error {
	return fc.setControlServerValue(keys.LogIngestServerURL, []byte(url))
}
func (fc *FlagController) LogIngestServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.LogIngestServerURL),
	).get(fc.getControlServerValue(keys.LogIngestServerURL))
}

// LogShippingLevel is the level at which logs should be shipped to the server
func (fc *FlagController) SetLogShippingLevel(level string) error {
	return fc.setControlServerValue(keys.LogShippingLevel, []byte(level))
}
func (fc *FlagController) SetLogShippingLevelOverride(value string, duration time.Duration) {
	ctx, span := observability.StartSpan(context.TODO())
	defer span.End()

	fc.overrideFlag(ctx, keys.LogShippingLevel, duration, value)
}
func (fc *FlagController) LogShippingLevel() string {
	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	const defaultLevel = "info"

	return NewStringFlagValue(
		WithOverrideString(fc.overrides[keys.LogShippingLevel]),
		WithDefaultString(defaultLevel),
		WithSanitizer(func(value string) string {
			value = strings.ToLower(value)
			switch value {
			case "debug", "warn", "info", "error":
				return value
			default:
				return defaultLevel
			}
		}),
	).get(fc.getControlServerValue(keys.LogShippingLevel))
}

func (fc *FlagController) SetTraceIngestServerURL(url string) error {
	return fc.setControlServerValue(keys.TraceIngestServerURL, []byte(url))
}
func (fc *FlagController) TraceIngestServerURL() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.TraceIngestServerURL),
	).get(fc.getControlServerValue(keys.TraceIngestServerURL))
}

func (fc *FlagController) SetDisableTraceIngestTLS(enabled bool) error {
	return fc.setControlServerValue(keys.DisableTraceIngestTLS, boolToBytes(enabled))
}
func (fc *FlagController) DisableTraceIngestTLS() bool {
	return NewBoolFlagValue(
		WithDefaultBool(fc.cmdLineOpts.DisableTraceIngestTLS),
	).get(fc.getControlServerValue(keys.DisableTraceIngestTLS))
}

func (fc *FlagController) SetInModernStandby(enabled bool) error {
	return fc.setControlServerValue(keys.InModernStandby, boolToBytes(enabled))
}
func (fc *FlagController) InModernStandby() bool {
	return NewBoolFlagValue(
		WithDefaultBool(false),
	).get(fc.getControlServerValue(keys.InModernStandby))
}

func (fc *FlagController) SetOsqueryHealthcheckStartupDelay(delay time.Duration) error {
	return fc.setControlServerValue(keys.OsqueryHealthcheckStartupDelay, durationToBytes(delay))
}
func (fc *FlagController) OsqueryHealthcheckStartupDelay() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.OsqueryHealthcheckStartupDelay,
		WithDefault(fc.cmdLineOpts.OsqueryHealthcheckStartupDelay),
		WithMin(0*time.Second),
		WithMax(1*time.Hour),
	).get(fc.getControlServerValue(keys.OsqueryHealthcheckStartupDelay))
}

func (fc *FlagController) LocalDevelopmentPath() string {
	return NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.LocalDevelopmentPath),
	).get(nil)
}

func (fc *FlagController) Identifier() string {
	identifier := NewStringFlagValue(
		WithDefaultString(fc.cmdLineOpts.Identifier),
	).get(nil)

	if strings.TrimSpace(identifier) == "" {
		identifier = launcher.DefaultLauncherIdentifier
	}

	return identifier
}

func (fc *FlagController) SetTableGenerateTimeout(interval time.Duration) error {
	return fc.setControlServerValue(keys.TableGenerateTimeout, durationToBytes(interval))
}
func (fc *FlagController) TableGenerateTimeout() time.Duration {
	return NewDurationFlagValue(fc.slogger, keys.TableGenerateTimeout,
		WithDefault(4*time.Minute),
		WithMin(30*time.Second),
		WithMax(10*time.Minute),
	).get(fc.getControlServerValue(keys.TableGenerateTimeout))
}
