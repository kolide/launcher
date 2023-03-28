package flags

import (
	"errors"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// FlagController is responsible for retrieving flag values from the appropriate sources,
// determining precedence, sanitizing flag values, and notifying observers of changes.
type FlagController struct {
	logger           log.Logger
	defaultValues    *FlagValues
	cmdLineValues    *FlagValues
	storedFlagValues *storedFlagValues
	sanitizer        *flagValueSanitizer
	overrideMutex    sync.RWMutex
	overrides        map[FlagKey]flagValueOverride
	observers        map[FlagsChangeObserver][]FlagKey
}

func NewFlagController(logger log.Logger, defaultValues *FlagValues, cmdLineValues *FlagValues, storedFlagValues *storedFlagValues, sanitizer *flagValueSanitizer) *FlagController {
	fc := &FlagController{
		logger:           logger,
		defaultValues:    defaultValues,
		cmdLineValues:    cmdLineValues,
		storedFlagValues: storedFlagValues,
		sanitizer:        sanitizer,
		overrides:        make(map[FlagKey]flagValueOverride),
		observers:        make(map[FlagsChangeObserver][]FlagKey),
	}

	return fc
}

func get[T any](fc *FlagController, key FlagKey) T {
	// If there is a temporary override, it takes precendence over anything else
	overrideValue, ok := getOverrideValue[T](fc, key)
	if ok {
		return overrideValue
	}

	// See if we can find a stored value to use
	storedValue, ok := getStoredValue[T](fc, key)
	if ok {
		return storedValue
	}

	// We were not able to find a suitable stored value, now cmd line flags take precedence
	cmdLineValue, ok := getCmdLineValue[T](fc, key)
	if ok {
		return cmdLineValue
	}

	// No suitable cmd line flag provided, now fallback to the default values
	return getDefaultValue[T](fc, key)
}

// getOverrideValue looks for an override value for the key and returns it, and true
// if an override value is not found, a zero value and false will be returned
func getOverrideValue[T any](fc *FlagController, key FlagKey) (T, bool) {
	override, ok := fc.overrides[key]
	if !ok {
		return *new(T), false
	}

	var t T
	var anyvalue any
	switch reflect.TypeOf(t).Kind() {
	case reflect.Bool:
		boolValue, ok := override.Value().(bool)
		if ok {
			anyvalue = boolValue
		}
	case reflect.Int64:
		int64Value, ok := override.Value().(int64)
		if ok {
			// Integers are sanitized to avoid unreasonable values
			if fc.sanitizer != nil {
				anyvalue = fc.sanitizer.Sanitize(key, int64Value)
			} else {
				anyvalue = int64Value
			}
		}
	case reflect.String:
		stringValue, ok := override.Value().(string)
		if ok {
			anyvalue = stringValue
		}
	default:
		level.Debug(fc.logger).Log("msg", "unsupported type of override flag", "type", reflect.TypeOf(t).Kind())
	}

	if anyvalue == nil {
		return *new(T), false
	}

	// Now we can get the underlying concrete value
	typedValue, ok := anyvalue.(T)
	if !ok {
		level.Debug(fc.logger).Log("msg", "override flag type assertion failed", "type", reflect.TypeOf(t).Kind())
		return *new(T), false
	}

	return typedValue, true
}

// getStoredValue looks for a stored value for the key and returns it, and true
// if a stored value is not found, a zero value and false will be returned
func getStoredValue[T any](fc *FlagController, key FlagKey) (T, bool) {
	if fc.storedFlagValues == nil {
		return *new(T), false
	}

	byteValue, exists := fc.storedFlagValues.Get(key)
	if !exists {
		return *new(T), false
	}

	var t T
	var anyvalue any
	// Determine the type that's being requested
	switch reflect.TypeOf(t).Kind() {
	case reflect.Bool:
		// Boolean flags are either present or not
		anyvalue = byteValue != nil
	case reflect.Int64:
		// Integers are stored as strings and need to be converted back
		int64Value, err := strconv.ParseInt(string(byteValue), 10, 64)
		if err != nil {
			level.Debug(fc.logger).Log("msg", "failed to convert stored integer flag value", "key", key, "err", err)
		}
		// Integers are sanitized to avoid unreasonable values
		if fc.sanitizer != nil {
			anyvalue = fc.sanitizer.Sanitize(key, int64Value)
		} else {
			anyvalue = int64Value
		}
	case reflect.String:
		anyvalue = string(byteValue)
	default:
		level.Debug(fc.logger).Log("msg", "unsupported type of stored flag", "type", reflect.TypeOf(t).Kind())
	}

	if anyvalue == nil {
		return *new(T), false
	}

	// Now we can get the underlying concrete value
	typedValue, ok := anyvalue.(T)
	if !ok {
		level.Debug(fc.logger).Log("msg", "stored flag type assertion failed", "type", reflect.TypeOf(t).Kind())
		return *new(T), false
	}

	return typedValue, true
}

// getCmdLineValue looks for a cmd line value for the key and returns it, and true
// if a cmd line value is not found, a zero value and false will be returned
func getCmdLineValue[T any](fc *FlagController, key FlagKey) (T, bool) {
	if fc.cmdLineValues == nil {
		return *new(T), false
	}

	value, exists := fc.cmdLineValues.Get(key)
	if !exists {
		return *new(T), false
	}

	typedValue, ok := value.(T)
	if !ok {
		var t T
		level.Debug(fc.logger).Log("msg", "cmd line flag type assertion failed", "type", reflect.TypeOf(t).Kind())
		return *new(T), false
	}

	return typedValue, true
}

// getDefaultValue looks for a default value for the key and returns it
// if a cmd line value is not found, a zero value is returned
func getDefaultValue[T any](fc *FlagController, key FlagKey) T {
	if fc.defaultValues == nil {
		return *new(T)
	}

	value, _ := fc.defaultValues.Get(key)
	typedValue, ok := value.(T)
	if !ok {
		var t T
		level.Debug(fc.logger).Log("msg", "default flag type assertion failed", "type", reflect.TypeOf(t).Kind())
		return *new(T)
	}

	return typedValue
}

func set[T any](fc *FlagController, key FlagKey, value T) error {
	if fc.storedFlagValues == nil {
		return errors.New("storedFlagValues is nil")
	}

	var anyvalue any = value
	byteValue, ok := anyvalue.([]byte)
	if ok {
		err := fc.storedFlagValues.Set(key, byteValue)
		if err != nil {
			level.Debug(fc.logger).Log("msg", "failed to set stored key", "key", key, "err", err)
			return err
		}
	}

	fc.notifyObservers(key)

	return nil
}

// Update bulk replaces agent flags and stores them.
// Observers will be notified of changed flags and deleted flags.
func (fc *FlagController) Update(pairs ...string) ([]string, error) {
	if fc.storedFlagValues == nil {
		return nil, errors.New("storedFlagValues is nil")
	}

	// Attempt to bulk replace the store with the key-values
	deletedKeys, err := fc.storedFlagValues.Update(pairs...)

	// Extract just the keys from the key-value pairs
	var updatedKeys []string
	for i := 0; i < len(pairs); i += 2 {
		updatedKeys = append(updatedKeys, pairs[i])
	}

	// Changed keys is the union of updated keys and deleted keys
	changedKeys := append(updatedKeys, deletedKeys...)

	// Now observers can be notified these keys have possibly changed
	fc.notifyObservers(toFlagKeys(changedKeys)...)

	return changedKeys, err
}

func (fc *FlagController) SetOverride(key FlagKey, value any, duration time.Duration) {
	// Always notify observers when overrides start, so they know to refresh.
	// Defering this before defering unlocking the mutex so that notifications occur outside of the critical section.
	defer fc.notifyObservers(key)

	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	override, ok := fc.overrides[key]
	if !ok {
		override = &Override{}
	}

	// Start a new override, or re-start an existing one with a new value, duration, and expiration
	override.Start(key, value, duration, fc.overrideExpired)

	if !ok {
		// Adding an override implicitly causes future flag retrievals to use the override until expiration
		fc.overrides[key] = override
	}
}

// overrideExpired removes an override and notifies observers of this change.
func (fc *FlagController) overrideExpired(key FlagKey) {
	// Always notify observers when overrides expire, so they know to refresh.
	// Defering this before defering unlocking the mutex so that notifications occur outside of the critical section.
	defer fc.notifyObservers(key)

	fc.overrideMutex.RLock()
	defer fc.overrideMutex.RUnlock()

	// Removing the override implictly allows the next value to take precedence
	fc.overrides[key] = nil
}

func (fc *FlagController) RegisterChangeObserver(observer FlagsChangeObserver, keys ...FlagKey) {
	fc.observers[observer] = append(fc.observers[observer], keys...)
}

// notifyObservers informs all observers of the keys that they have changed.
func (fc *FlagController) notifyObservers(keys ...FlagKey) {
	for observer, observedKeys := range fc.observers {
		changedKeys := intersection(observedKeys, keys)

		if len(changedKeys) > 0 {
			observer.FlagsChanged(changedKeys...)
		}
	}
}

func (fc *FlagController) SetDesktopEnabled(enabled bool) error {
	return set(fc, DesktopEnabled, enabled)
}
func (fc *FlagController) DesktopEnabled() bool {
	return get[bool](fc, DesktopEnabled)
}

func (fc *FlagController) SetDebugServerData(debug bool) error {
	return set(fc, DebugServerData, debug)
}
func (fc *FlagController) DebugServerData() bool {
	return get[bool](fc, DebugServerData)
}

func (fc *FlagController) SetForceControlSubsystems(force bool) error {
	return set(fc, ForceControlSubsystems, force)
}
func (fc *FlagController) ForceControlSubsystems() bool {
	return get[bool](fc, ForceControlSubsystems)
}

func (fc *FlagController) SetControlServerURL(url string) error {
	return set(fc, ControlServerURL, url)
}
func (fc *FlagController) ControlServerURL() string {
	return get[string](fc, ControlServerURL)
}

func (fc *FlagController) SetControlRequestInterval(interval time.Duration) error {
	return set(fc, ControlRequestInterval, interval)
}
func (fc *FlagController) ControlRequestInterval() time.Duration {
	return get[time.Duration](fc, ControlRequestInterval)
}

func (fc *FlagController) SetDisableControlTLS(disabled bool) error {
	return set(fc, DisableControlTLS, disabled)
}
func (fc *FlagController) DisableControlTLS() bool {
	return get[bool](fc, DisableControlTLS)
}

func (fc *FlagController) SetInsecureControlTLS(disabled bool) error {
	return set(fc, InsecureControlTLS, disabled)
}
func (fc *FlagController) InsecureControlTLS() bool {
	return get[bool](fc, InsecureControlTLS)
}
