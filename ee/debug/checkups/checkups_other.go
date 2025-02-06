//go:build !windows
// +build !windows

package checkups

func (s Status) Emoji() string {
	switch s {
	case Informational:
		return " "
	case Passing:
		return "✅"
	case Warning:
		return "⚠️"
	case Failing:
		return "❌"
	case Erroring:
		return "❌"
	case Unknown:
		return "? "
	default:
		return " "
	}
}
