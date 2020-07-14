package tablehelpers

import (
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go/plugin/table"
)

type constraintOptions struct {
	defaults          []string
	allowedCharacters string
	logger            log.Logger
}

type GetConstraintOpts func(*constraintOptions)

// WithLogger sets the logger to use
func WithLogger(logger log.Logger) GetConstraintOpts {
	return func(co *constraintOptions) {
		co.logger = logger
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

// GetConstraints returns a []string of the constraint expressions on
// a column. It's meant for the common, simple, usecase of iterating over them.
func GetConstraints(queryContext table.QueryContext, columnName string, opts ...GetConstraintOpts) []string {

	co := &constraintOptions{
		logger: log.NewNopLogger(),
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
		if !co.OnlyAllowedCharacters(c.Expression) {
			level.Info(co.logger).Log(
				"msg", "Disallowed character in expression",
				"column", columnName,
				"expression", c.Expression,
			)
			continue
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
