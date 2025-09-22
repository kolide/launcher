package tablehelpers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
)

type constraintOptions struct {
	allowedCharacters string
	allowedValues     []string
	defaults          []string
	slogger           *slog.Logger
}

type GetConstraintOpts func(*constraintOptions)

// WithLogger sets the logger to use
func WithSlogger(slogger *slog.Logger) GetConstraintOpts {
	return func(co *constraintOptions) {
		co.slogger = slogger
	}
}

// WithDefaults sets the defaults to use if no constraints were
// specified. Note that this does not apply if there were constraints,
// which were invalidated.
func WithDefaults(defaults ...string) GetConstraintOpts {
	return func(co *constraintOptions) {
		co.defaults = append(co.defaults, defaults...)
	}
}

func WithAllowedCharacters(allowed string) GetConstraintOpts {
	return func(co *constraintOptions) {
		co.allowedCharacters = allowed
	}
}

func WithAllowedValues(allowed []string) GetConstraintOpts {
	return func(co *constraintOptions) {
		co.allowedValues = allowed
	}
}

// GetConstraints returns a []string of the constraint expressions on
// a column. It's meant for the common, simple, usecase of iterating over them.
func GetConstraints(queryContext table.QueryContext, columnName string, opts ...GetConstraintOpts) []string {

	co := &constraintOptions{
		slogger: multislogger.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(co)
	}

	q, ok := queryContext.Constraints[columnName]
	if !ok || len(q.Constraints) == 0 {
		return co.defaults
	}

	constraintSet := make(map[string]struct{})

	for _, c := range q.Constraints {
		// No point in checking allowed characters, if we have an allowedValues. Just use it.
		if len(co.allowedValues) == 0 && !co.OnlyAllowedCharacters(c.Expression) {
			co.slogger.Log(context.TODO(), slog.LevelInfo,
				"disallowed character in expression",
				"column", columnName,
				"expression", c.Expression,
			)

			continue
		}

		if len(co.allowedValues) > 0 {
			skip := true
			for _, v := range co.allowedValues {
				if v == c.Expression {
					skip = false
					break
				}
			}

			if skip {
				co.slogger.Log(context.TODO(), slog.LevelInfo,
					"disallowed character in expression",
					"column", columnName,
					"expression", c.Expression,
				)
				continue
			}
		}

		// empty struct is less ram than bool would be
		constraintSet[c.Expression] = struct{}{}
	}

	constraints := make([]string, len(constraintSet))

	i := 0
	for key := range constraintSet {
		constraints[i] = key
		i++
	}

	return constraints
}

func (co *constraintOptions) OnlyAllowedCharacters(input string) bool {
	if co.allowedCharacters == "" {
		return true
	}

	for _, char := range input {
		if !strings.ContainsRune(co.allowedCharacters, char) {
			return false
		}
	}
	return true
}
