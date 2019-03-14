// +build !windows

package packaging

// formatVersion formats the version. This is specific to windows. It
// may show up elsewhere later.
//
// Windows packages must confirm to W.X.Y.Z, so we convert our git
// format to that.
func formatVersion(rawVersion string) (string, error) {
	return rawVersion, nil
}
