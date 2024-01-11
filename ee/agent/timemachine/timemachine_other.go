//go:build !darwin
// +build !darwin

package timemachine

import (
	"context"

	"github.com/kolide/launcher/ee/agent/types"
)

// AddExclusions adds specific launcher time machine exclusions for
// darwin and is noop for other oses
func AddExclusions(_ context.Context, _ types.Knapsack) {
	// do nothing, no time machine on non darwin
}
