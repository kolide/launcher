package flags

import (
	"math"
	"strconv"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/agent/flags/keys"
)

type float64Option func(*float64FlagValue)

func WithFloat64ValueOverride(override FlagValueOverride) float64Option {
	return func(f *float64FlagValue) {
		f.override = override
	}
}

func WithFloat64ValueDefault(defaultVal float64) float64Option {
	return func(f *float64FlagValue) {
		f.defaultVal = defaultVal
	}
}

func WithFloat64ValueMin(min float64) float64Option {
	return func(f *float64FlagValue) {
		f.min = min
	}
}

func WithFloat64ValueMax(max float64) float64Option {
	return func(f *float64FlagValue) {
		f.max = max
	}
}

type float64FlagValue struct {
	logger     log.Logger
	key        keys.FlagKey
	override   FlagValueOverride
	defaultVal float64
	min        float64
	max        float64
}

func NewFloat64FlagValue(logger log.Logger, key keys.FlagKey, opts ...float64Option) *float64FlagValue {
	f := &float64FlagValue{
		logger: logger,
		key:    key,
		min:    -1 * math.MaxFloat64,
		max:    math.MaxFloat64,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

func (f *float64FlagValue) get(controlServerValue []byte) float64 {
	float64Value := f.defaultVal
	if controlServerValue != nil {
		// Control server provided floats are stored as strings and need to be converted back
		var err error
		parsedFloat, err := strconv.ParseFloat(string(controlServerValue), 64)
		if err == nil {
			float64Value = parsedFloat
		} else {
			level.Debug(f.logger).Log("msg", "failed to convert stored float flag value", "key", f.key, "err", err)
		}
	}

	if f.override != nil && f.override.Value() != nil {
		// An override was provided, if it's valid let it take precedence
		value, ok := f.override.Value().(float64)
		if ok {
			float64Value = value
		}
	}

	// Integers are sanitized to avoid unreasonable values
	return clampFloat64Value(float64Value, f.min, f.max)
}

// clampValue returns a value that is clamped to be within the range defined by min and max.
func clampFloat64Value(value float64, min, max float64) float64 {
	switch {
	case value < min:
		return min
	case value > max:
		return max
	default:
		return value
	}
}

func float64ToBytes(f float64) []byte {
	return []byte(strconv.FormatFloat(f, 'f', -1, 64))
}
