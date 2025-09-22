package flags

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
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

func WithMin(minimum time.Duration) durationOption {
	return func(d *durationFlagValue) {
		d.min = int64(minimum)
	}
}

func WithMax(maximum time.Duration) durationOption {
	return func(d *durationFlagValue) {
		d.max = int64(maximum)
	}
}

type durationFlagValue struct {
	slogger    *slog.Logger
	key        keys.FlagKey
	override   FlagValueOverride
	defaultVal int64
	min        int64
	max        int64
}

func NewDurationFlagValue(slogger *slog.Logger, key keys.FlagKey, opts ...durationOption) *durationFlagValue {
	d := &durationFlagValue{
		slogger: slogger,
		key:     key,
		min:     math.MinInt64,
		max:     math.MaxInt64,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

func (d *durationFlagValue) get(controlServerValue []byte) time.Duration {
	int64Value := d.defaultVal

	if controlServerValue != nil {
		int64Value = d.parseControlServerValue(controlServerValue)
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

// parseControlServerValue attempts to parse the control server value as either a duration string or nanoseconds
func (d *durationFlagValue) parseControlServerValue(controlServerValue []byte) int64 {
	valueStr := string(controlServerValue)

	// First try to parse as a duration string (e.g., "4s", "10m")
	if parsedDuration, err := time.ParseDuration(valueStr); err == nil {
		return int64(parsedDuration)
	}

	// Fall back to parsing as nanoseconds for backward compatibility
	if parsedInt, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
		return parsedInt
	}

	// Both parsing attempts failed, log and return default
	d.slogger.Log(context.TODO(), slog.LevelDebug,
		"failed to convert stored duration flag value",
		"key", d.key,
		"value", valueStr,
	)
	return d.defaultVal
}

// clampValue returns a value that is clamped to be within the range defined by min and max.
func clampValue(value int64, minimum, maximum int64) int64 {
	switch {
	case value < minimum:
		return minimum
	case value > maximum:
		return maximum
	default:
		return value
	}
}

func durationToBytes(duration time.Duration) []byte {
	return []byte(strconv.FormatInt(int64(duration), 10))
}
