//go:build windows
// +build windows

package checkups

// Emoji returns the Windows-friendly symbol for the given status. Powershell will not
// display actual emojis.
func (s Status) Emoji() string {
	switch s {
	case Informational:
		return " "
	case Passing:
		return "OK "
	case Warning:
		return "! "
	case Failing:
		return "X "
	case Erroring:
		return "X "
	case Unknown:
		return "? "
	default:
		return " "
	}
}
