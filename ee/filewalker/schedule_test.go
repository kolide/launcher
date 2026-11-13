package filewalker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// dueStrict should return config names immediately ready to walk.
func Test_dueStrict(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_000_000, 0)
	cfgs := map[string]filewalkConfig{
		"never_walked": {WalkInterval: duration(time.Hour)},
		"future":       {WalkInterval: duration(time.Hour)},
		"not_due":      {WalkInterval: duration(time.Hour)},
		"elapsed":      {WalkInterval: duration(time.Hour)},
	}
	lastWalks := map[string]time.Time{
		"future":  now.Add(24 * time.Hour), // clock moved back
		"not_due": now.Add(-59 * time.Minute),
		"elapsed": now.Add(-time.Hour),
	}

	require.ElementsMatch(t, []string{"elapsed", "future", "never_walked"}, dueStrict(now, cfgs, lastWalks))
}
