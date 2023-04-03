package flags

import (
	"math"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
)

type durationOption func(*durationFlagValue)

func WithOverride(override FlagValueOverride) durationOption {
	return func(d *durationFlagValue) {
		d.override = override
	}
}

func WithDefault(defaultVal time.Duration) durationOption {
	return func(d *durationFlagValue) {
		d.defaultVal = int64(defaultVal)
	}
}

func WithMin(min time.Duration) durationOption {
	return func(d *durationFlagValue) {
		d.min = int64(min)
	}
}

func WithMax(max time.Duration) durationOption {
	return func(d *durationFlagValue) {
		d.max = int64(max)
	}
}

type durationFlagValue struct {
	logger     log.Logger
	key        keys.FlagKey
	override   FlagValueOverride
	defaultVal int64
	min        int64
	max        int64
}

func NewDurationFlagValue(logger log.Logger, key keys.FlagKey, opts ...durationOption) *durationFlagValue {
	d := &durationFlagValue{
		logger: logger,
		key:    key,
		min:    math.MinInt64,
		max:    math.MaxInt64,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

func (d *durationFlagValue) get(controlServerValue []byte) time.Duration {
	int64Value := d.defaultVal
	if controlServerValue != nil {
		// Control server provided integers are stored as strings and need to be converted back
		var err error
		parsedInt, err := strconv.ParseInt(string(controlServerValue), 10, 64)
		if err == nil {
			int64Value = parsedInt
		} else {
			level.Debug(d.logger).Log("msg", "failed to convert stored duration flag value", "key", d.key, "err", err)
		}
	}

	if d.override != nil && d.override.Value() != nil {
		// An override was provided, if it's valid let it take precedence
		value, ok := d.override.Value().(time.Duration)
		if ok {
			int64Value = int64(value)
		}
	}

	// Integers are sanitized to avoid unreasonable values
	int64Value = clampValue(int64Value, d.min, d.max)
	return time.Duration(int64Value)
}

// clampValue returns a value that is clamped to be within the range defined by min and max.
func clampValue(value int64, min, max int64) int64 {
	switch {
	case value < min:
		return min
	case value > max:
		return max
	default:
		return value
	}
}

func durationToBytes(duration time.Duration) []byte {
	return []byte(strconv.FormatInt(int64(duration), 10))
}
