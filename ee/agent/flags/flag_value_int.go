package flags

import (
	"context"
	"log/slog"
	"math"
	"strconv"

	"github.com/kolide/launcher/ee/agent/flags/keys"
)

type intOption func(*intFlagValue)

func WithIntValueOverride(override FlagValueOverride) intOption {
	return func(f *intFlagValue) {
		f.override = override
	}
}

func WithIntValueDefault(defaultVal int) intOption {
	return func(i *intFlagValue) {
		i.defaultVal = defaultVal
	}
}

func WithIntValueMin(minimum int) intOption {
	return func(i *intFlagValue) {
		i.min = minimum
	}
}

func WithIntValueMax(maximum int) intOption {
	return func(i *intFlagValue) {
		i.max = maximum
	}
}

type intFlagValue struct {
	slogger    *slog.Logger
	key        keys.FlagKey
	override   FlagValueOverride
	defaultVal int
	min        int
	max        int
}

func NewIntFlagValue(slogger *slog.Logger, key keys.FlagKey, opts ...intOption) *intFlagValue {
	i := &intFlagValue{
		slogger: slogger,
		key:     key,
		min:     -1 * math.MaxInt,
		max:     math.MaxInt,
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

func (i *intFlagValue) get(controlServerValue []byte) int {
	intValue := i.defaultVal
	if controlServerValue != nil {
		// Control server provided ints are stored as strings and need to be converted back
		var err error
		parsedInt, err := strconv.Atoi(string(controlServerValue))
		if err == nil {
			intValue = parsedInt
		} else {
			i.slogger.Log(context.TODO(), slog.LevelDebug,
				"failed to convert stored int flag value",
				"key", i.key,
				"err", err,
			)
		}
	}

	if i.override != nil && i.override.Value() != nil {
		// An override was provided, if it's valid let it take precedence
		value, ok := i.override.Value().(int)
		if ok {
			intValue = value
		}
	}

	// Integers are sanitized to avoid unreasonable values
	return clampIntValue(intValue, i.min, i.max)
}

// clampValue returns a value that is clamped to be within the range defined by min and max.
func clampIntValue(value int, minimum, maximum int) int {
	switch {
	case value < minimum:
		return minimum
	case value > maximum:
		return maximum
	default:
		return value
	}
}

func intToBytes(i int) []byte {
	return []byte(strconv.Itoa(i))
}
