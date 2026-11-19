package filewalker

import (
	"slices"
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

func Test_dueHeartbeat(t *testing.T) {
	t.Parallel()

	now := time.Time{}.Add(24_000 * time.Hour)
	hourAgo := now.Add(-time.Hour)
	for _, tt := range []struct {
		name      string
		intervals map[string]time.Duration
		lastWalks map[string]time.Time
		due       []string
	}{
		{
			name: "no cfgs",
		},
		{
			name:      "never walked",
			intervals: map[string]time.Duration{"a": time.Hour},
			due:       []string{"a"},
		},
		{
			name:      "two never walked",
			intervals: map[string]time.Duration{"a": time.Hour, "b": 4 * time.Hour},
			due:       []string{"a", "b"},
		},
		{
			name:      "one due",
			intervals: map[string]time.Duration{"a": time.Hour},
			lastWalks: map[string]time.Time{"a": now.Add(-61 * time.Minute)},
			due:       []string{"a"},
		},
		{
			name:      "one not due",
			intervals: map[string]time.Duration{"a": time.Hour},
			lastWalks: map[string]time.Time{"a": now.Add(-time.Hour + time.Minute)},
		},
		{
			name:      "two with one not close to due",
			intervals: map[string]time.Duration{"due": time.Hour, "notdue": 4 * time.Hour},
			lastWalks: map[string]time.Time{
				"due":    now.Add(-90 * time.Minute),
				"notdue": now.Add(-30 * time.Minute),
			},
			due: []string{"due"},
		},
		{
			name:      "two cfgs and one gets coalesced",
			intervals: map[string]time.Duration{"due": time.Hour, "coalesced": 2 * time.Hour},
			lastWalks: map[string]time.Time{"due": hourAgo, "coalesced": hourAgo.Add(-time.Minute)},
			due:       []string{"due", "coalesced"},
		},
		{
			name:      "almost due folds into heartbeat",
			intervals: map[string]time.Duration{"due": time.Hour, "almost": 4 * time.Hour},
			lastWalks: map[string]time.Time{"due": hourAgo, "almost": now.Add(-4*time.Hour + 2*time.Minute)},
			due:       []string{"due", "almost"},
		},
		{
			name:      "two intervals just due",
			intervals: map[string]time.Duration{"a": time.Hour, "b": 3 * time.Hour},
			lastWalks: map[string]time.Time{"a": hourAgo, "b": hourAgo.Add(-time.Hour)},
			due:       []string{"a", "b"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfgs := make(map[string]filewalkConfig, len(tt.intervals))
			for name, interval := range tt.intervals {
				cfgs[name] = filewalkConfig{WalkInterval: duration(interval)}
			}
			require.ElementsMatch(t, tt.due, dueHeartbeat(now, cfgs, tt.lastWalks))
		})
	}
}

func Test_dueHeartbeat_edgeCases(t *testing.T) {
	t.Parallel()

	now := time.Time{}.Add(24_000 * time.Hour)
	for _, tt := range []struct {
		name      string
		intervals map[string]time.Duration
		lastWalks map[string]time.Time
		due       []string
		notDue    []string
	}{
		{
			name:      "zero interval",
			intervals: map[string]time.Duration{"zero": 0, "b": time.Hour},
			lastWalks: map[string]time.Time{"zero": now.Add(-5 * time.Minute), "b": now.Add(-5 * time.Minute)},
			due:       []string{"zero"},
			notDue:    []string{"b"},
		},
		{
			name:      "negative interval",
			intervals: map[string]time.Duration{"negative": -time.Hour, "b": time.Hour},
			lastWalks: map[string]time.Time{"negative": now.Add(-5 * time.Minute), "b": now.Add(-5 * time.Minute)},
			due:       []string{"negative"},
			notDue:    []string{"b"},
		},
		{
			name:      "future last walk",
			intervals: map[string]time.Duration{"future": time.Hour, "b": time.Hour},
			lastWalks: map[string]time.Time{"future": now.Add(24 * time.Hour), "b": now.Add(-5 * time.Minute)},
			due:       []string{"future"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfgs := make(map[string]filewalkConfig, len(tt.intervals))
			for name, interval := range tt.intervals {
				cfgs[name] = filewalkConfig{WalkInterval: duration(interval)}
			}
			var names []string
			require.NotPanics(t, func() { names = dueHeartbeat(now, cfgs, tt.lastWalks) })
			require.Subset(t, names, tt.due)
			for _, name := range tt.notDue {
				require.NotContains(t, names, name)
			}
		})
	}
}

// Tied cfgs for smallest interval should converge and walk together.
func Test_dueHeartbeat_tiedHeartbeatsConverge(t *testing.T) {
	t.Parallel()

	now := time.Time{}.Add(24_000 * time.Hour)
	cfgs := map[string]filewalkConfig{
		"a": {WalkInterval: duration(time.Hour)},
		"b": {WalkInterval: duration(time.Hour)},
		"c": {WalkInterval: duration(time.Hour)},
	}
	lastWalks := map[string]time.Time{
		"a": now.Add(-61 * time.Minute),
		"b": now.Add(-10 * time.Minute),
		"c": now.Add(-time.Hour),
	}
	runs := map[string]int{}
	var together bool
	for end := now.Add(3 * time.Hour); now.Before(end); now = now.Add(walkCheckInterval) {
		names := dueHeartbeat(now, cfgs, lastWalks)
		together = together || len(names) == len(cfgs)
		for _, name := range names {
			lastWalks[name] = now
			runs[name]++
		}
	}

	require.Len(t, runs, len(cfgs))
	require.True(t, together)
}

func Test_dueHeartbeat_coalesces(t *testing.T) {
	t.Parallel()

	epoch := time.Unix(0, 0).Add(24_000 * time.Hour)
	cfgs := map[string]filewalkConfig{
		"a": {WalkInterval: duration(2 * time.Hour)},
		"b": {WalkInterval: duration(9 * time.Hour)},
	}
	lastWalks := map[string]time.Time{}
	runs := map[string]int{}
	for now := epoch; now.Before(epoch.Add(24 * time.Hour)); now = now.Add(walkCheckInterval) {
		names := dueHeartbeat(now, cfgs, lastWalks)
		if slices.Contains(names, "b") {
			require.Contains(t, names, "a", "b ran without a at %s", now.Sub(epoch))
		}
		for _, name := range names {
			lastWalks[name] = now
			runs[name]++
		}
	}

	require.Equal(t, map[string]int{"a": 12, "b": 3}, runs)
}
