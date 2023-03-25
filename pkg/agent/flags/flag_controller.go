package flags

import (
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
	observers        map[FlagKey][]FlagsChangeObserver
}

func NewFlagController(logger log.Logger, defaultValues *AnyFlagValues, cmdLineValues *AnyFlagValues, storedFlagValues *storedFlagValues) *FlagController {
	f := &FlagController{
		logger:           logger,
		defaultValues:    defaultValues,
		cmdLineValues:    cmdLineValues,
		storedFlagValues: storedFlagValues,
		observers:        make(map[FlagKey][]FlagsChangeObserver),
	}

	return f
}

func get[T any](c *FlagController, key FlagKey) T {
	var t T
	if c.storedFlagValues != nil {
		byteValue, exists := c.storedFlagValues.Get(key)
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
					level.Debug(c.logger).Log("msg", "failed to convert stored integer flag value", "key", key, "err", err)
				}
				anyvalue = int64Value
			case reflect.String:
				anyvalue = string(byteValue)
			default:
				level.Debug(c.logger).Log("msg", "unsupported type of stored flag", "type", reflect.TypeOf(t).Kind())
			}

			if anyvalue != nil {
				// Now we can get the underlying concrete value
				typedValue, ok := anyvalue.(T)
				if ok {
					return typedValue // TODO sanitize
				} else {
					level.Debug(c.logger).Log("msg", "stored flag type assertion failed", "type", reflect.TypeOf(t).Kind())
				}
			}
		}
	}

	// We were not able to find a suitable stored key, now cmd line options take precedence
	if c.cmdLineValues != nil {
		value, exists := c.cmdLineValues.Get(key)
		if exists {
			typedValue, ok := value.(T)
			if ok {
				return typedValue // TODO sanitize
			} else {
				level.Debug(c.logger).Log("msg", "cmd line flag type assertion failed", "type", reflect.TypeOf(t).Kind())
			}
		}
	}

	// No suitable cmd line option provided, now fallback to the default values
	if c.defaultValues != nil {
		value, _ := c.defaultValues.Get(key)
		typedValue, ok := value.(T)
		if ok {
			return typedValue
		} else {
			level.Debug(c.logger).Log("msg", "default flag type assertion failed", "type", reflect.TypeOf(t).Kind())
		}
	}

	// If all else fails, return the zero value for the type
	return *new(T)
}

func set[T any](c *FlagController, key FlagKey, value T) error {

	// TODO sanitize

	var anyvalue any = value
	byteValue, ok := anyvalue.([]byte)
	if ok {
		err := c.storedFlagValues.Set(key, byteValue)
		if err != nil {
			level.Debug(c.logger).Log("msg", "failed to set stored key", "key", key, "err", err)
			return err
		}
	}

	c.notifyObservers(key)

	return nil
}

func (f *FlagController) RegisterChangeObserver(observer FlagsChangeObserver, keys ...FlagKey) {
	for _, key := range keys {
		f.observers[key] = append(f.observers[key], observer)
	}
}

func (f *FlagController) notifyObservers(keys ...FlagKey) {
	for _, key := range keys {
		if observers, ok := f.observers[key]; ok {
			for _, observer := range observers {
				observer.FlagsChanged(key)
			}
		}
	}
}

func (f *FlagController) SetDesktopEnabled(enabled bool) error {
	return set(f, DesktopEnabled, enabled)
}
func (f *FlagController) DesktopEnabled() bool {
	return get[bool](f, DesktopEnabled)
}

func (f *FlagController) SetDebugServerData(debug bool) error {
	return set(f, DebugServerData, debug)
}
func (f *FlagController) DebugServerData() bool {
	return get[bool](f, DebugServerData)
}

func (f *FlagController) SetForceControlSubsystems(force bool) error {
	return set(f, ForceControlSubsystems, force)
}
func (f *FlagController) ForceControlSubsystems() bool {
	return get[bool](f, ForceControlSubsystems)
}

func (f *FlagController) SetControlServerURL(url string) error {
	return set(f, ControlServerURL, url)
}
func (f *FlagController) ControlServerURL() string {
	return get[string](f, ControlServerURL)
}

func (f *FlagController) SetControlRequestInterval(interval time.Duration) error {
	return set(f, ControlRequestInterval, interval)
}
func (f *FlagController) ControlRequestInterval() time.Duration {
	return get[time.Duration](f, ControlRequestInterval)
}

func (f *FlagController) SetDisableControlTLS(disabled bool) error {
	return set(f, DisableControlTLS, disabled)
}
func (f *FlagController) DisableControlTLS() bool {
	return get[bool](f, DisableControlTLS)
}

func (f *FlagController) SetInsecureControlTLS(disabled bool) error {
	return set(f, InsecureControlTLS, disabled)
}
func (f *FlagController) InsecureControlTLS() bool {
	return get[bool](f, InsecureControlTLS)
}
