// Package tuf provides target resolution helpers for TUF metadata.
package tuf

// ArchForPlatform returns the TUF arch string for the given platform.
// Darwin uses "universal"; others use the provided arch.
func ArchForPlatform(platform, arch string) string {
	if platform == "darwin" {
		return "universal"
	}
	return arch
}
