package filewalker

import "time"

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
