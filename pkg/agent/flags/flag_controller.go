package flags

import (
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// FlagController is responsible for retrieving flag values from the appropriate sources,
// determining precedence, sanitizing flag values, and notifying observers of changes.
type FlagController struct {
	logger           log.Logger
	defaultValues    *AnyFlagValues
	cmdLineValues    *AnyFlagValues
	storedFlagValues *storedFlagValues
	sanitizer        *flagValueSanitizer
	observers        map[FlagsChangeObserver][]FlagKey
}

func NewFlagController(logger log.Logger, defaultValues *AnyFlagValues, cmdLineValues *AnyFlagValues, storedFlagValues *storedFlagValues, sanitizer *flagValueSanitizer) *FlagController {
	fc := &FlagController{
		logger:           logger,
		defaultValues:    defaultValues,
		cmdLineValues:    cmdLineValues,
		storedFlagValues: storedFlagValues,
		sanitizer:        sanitizer,
		observers:        make(map[FlagsChangeObserver][]FlagKey),
	}

	return fc
}

func get[T any](fc *FlagController, key FlagKey) T {
	var t T
	if fc.storedFlagValues != nil {
		byteValue, exists := fc.storedFlagValues.Get(key)
		if exists {
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

			if anyvalue != nil {
				// Now we can get the underlying concrete value
				typedValue, ok := anyvalue.(T)
				if ok {
					return typedValue
				} else {
					level.Debug(fc.logger).Log("msg", "stored flag type assertion failed", "type", reflect.TypeOf(t).Kind())
				}
			}
		}
	}

	// We were not able to find a suitable stored key, now cmd line options take precedence
	if fc.cmdLineValues != nil {
		value, exists := fc.cmdLineValues.Get(key)
		if exists {
			typedValue, ok := value.(T)
			if ok {
				return typedValue // TODO sanitize
			} else {
				level.Debug(fc.logger).Log("msg", "cmd line flag type assertion failed", "type", reflect.TypeOf(t).Kind())
			}
		}
	}

	// No suitable cmd line option provided, now fallback to the default values
	if fc.defaultValues != nil {
		value, _ := fc.defaultValues.Get(key)
		typedValue, ok := value.(T)
		if ok {
			return typedValue
		} else {
			level.Debug(fc.logger).Log("msg", "default flag type assertion failed", "type", reflect.TypeOf(t).Kind())
		}
	}

	// If all else fails, return the zero value for the type
	return *new(T)
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

func toFlagKeys(s []string) []FlagKey {
	f := make([]FlagKey, len(s))
	for i, v := range s {
		f[i] = FlagKey(v)
	}
	return f
}

// Returns the intersection of FlagKeys; keys which exist in both a and b.
func intersection(a, b []FlagKey) []FlagKey {
	m := make(map[FlagKey]bool)
	var result []FlagKey

	for _, element := range a {
		m[element] = true
	}

	for _, element := range b {
		if m[element] {
			result = append(result, element)
		}
	}

	return result
}

func (fc *FlagController) RegisterChangeObserver(observer FlagsChangeObserver, keys ...FlagKey) {
	fc.observers[observer] = append(fc.observers[observer], keys...)
}

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
