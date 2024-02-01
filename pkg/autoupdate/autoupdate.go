// Package autoupdate provides some limited functionality related to
// autoupdating. The majority of the functionality now lives in
// ee/tuf.
package autoupdate

// UpdateChannel determines the TUF target for a Updater.
// The Default UpdateChannel is Stable.
type UpdateChannel string

const (
	Stable  UpdateChannel = "stable"
	Alpha   UpdateChannel = "alpha"
	Beta    UpdateChannel = "beta"
	Nightly UpdateChannel = "nightly"
)

func (c UpdateChannel) String() string {
	return string(c)
}

func SanitizeUpdateChannel(value string) string {
	switch UpdateChannel(value) {
	case Stable, Alpha, Beta, Nightly:
		return value
	}
	// Fallback to stable if invalid channel
	return Stable.String()
}

const (
	DefaultMirror = "https://dl.kolide.co"
)
