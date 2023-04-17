package flags

import (
	"sort"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/types"
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
	sort.Strings(changedKeys)

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

func (fc *FlagController) SetDesktopEnabled(enabled bool) error {
	return fc.set(keys.DesktopEnabled, boolToBytes(enabled))
}
func (fc *FlagController) DesktopEnabled() bool {
	return NewBoolFlagValue(WithDefaultBool(false)).get(fc.getControlServerValue(keys.DesktopEnabled))
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
