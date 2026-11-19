package filewalker

import (
	"maps"
	"math"
	"slices"
	"time"
)

// The sole non-default due strategy, dueStrict
const dueStrategyStrict = "strict"

// dueStrict returns the filewalkConfig names which are immediately due for walking. Anything
// last walked in the future, not walked, or failing to parse when it walked are included.
func dueStrict(now time.Time, cfgs map[string]filewalkConfig, lastWalks map[string]time.Time) []string {
	var names []string
	for name, cfg := range cfgs {
		elapsed := now.Sub(lastWalks[name])
		if elapsed < 0 || elapsed >= time.Duration(cfg.WalkInterval) {
			names = append(names, name)
		}
	}
	return names
}

// dueHeartbeat returns filewalkConfig names to walk, trading extra cfg evals for fewer walks.
//
// The smallest interval is the heartbeat. Walks happen only when the heartbeat is due, and include every
// cfg due before the next heartbeat (e.g., 4 configs 2h 4h 5h 9h, heartbeat is 2h, cfgs respectively evaled
// every 2h 4h 4h 8h). New cfgs and cfgs last walked in the future run immediately (see mustRunNow docs).
//
// Intended for cfgs with heavily overlapping and dense trees, where walking costs far more than
// evaluating extra cfgs.
func dueHeartbeat(now time.Time, cfgs map[string]filewalkConfig, lastWalks map[string]time.Time) []string {
	// whatever needs ran due to edge cases
	names := mustRunNow(now, cfgs, lastWalks)
	// smallest interval we tick on
	heartbeat := time.Duration(math.MaxInt64)
	// last time a heartbeat ran
	lastHeartbeat := time.Time{}
	for name, cfg := range cfgs {
		interval := time.Duration(cfg.WalkInterval)
		heartbeat = min(heartbeat, interval)
		if heartbeat == interval {
			// if multiple heartbeat cfgs, it's suboptimal but simple to just take one
			lastHeartbeat = lastWalks[name]
		}
	}

	// only run on heartbeats or for new walks
	if lastHeartbeat.Add(heartbeat).After(now) && len(names) == 0 {
		return nil
	}
	nextHeartbeat := now.Add(heartbeat)

	for name, cfg := range cfgs {
		walkDeadline := lastWalks[name].Add(time.Duration(cfg.WalkInterval))

		// Coalesce by running anything due before the next heartbeat.
		// !Time.After over Time.Before causes coalescence in edge cases
		// where runs finish in parallel.
		if !walkDeadline.After(nextHeartbeat) {
			names[name] = struct{}{}
		}
	}

	return slices.Collect(maps.Keys(names))
}

// mustRunNow returns cfg names that must be ran due to clock drift or never
// having ran.
func mustRunNow(now time.Time, cfgs map[string]filewalkConfig, lastWalks map[string]time.Time) map[string]struct{} {
	names := make(map[string]struct{})
	for name := range cfgs {
		last, hasRun := lastWalks[name]
		if !hasRun || last.After(now) {
			names[name] = struct{}{}
		}
	}
	return names
}
