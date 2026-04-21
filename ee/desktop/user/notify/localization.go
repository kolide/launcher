package notify

import (
	"encoding/json"
	"os"

	"github.com/kolide/launcher/v2/ee/agent/types"
)

// learnMoreFallback is the English default used when no localized string is
// available (path empty, file missing/corrupt, locale not present, or key empty).
const learnMoreFallback = "Learn More"

// loadLocalizationData reads the localization file written by the runner.
// Returns a zero-value LocalizationData on any error so callers can fall back
// to defaults without surfacing errors at the UI layer.
func loadLocalizationData(path string) types.LocalizationData {
	if path == "" {
		return types.LocalizationData{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return types.LocalizationData{}
	}

	var ld types.LocalizationData
	if err := json.Unmarshal(data, &ld); err != nil {
		return types.LocalizationData{}
	}

	return ld
}

// learnMoreLabel returns the localized "Learn More" action label for the
// configured locale, or the English default if no translation is available.
func learnMoreLabel(path string) string {
	ld := loadLocalizationData(path)
	if t, ok := ld.Translations[ld.Locale]; ok && t.Notifications.Actions.LearnMore != "" {
		return t.Notifications.Actions.LearnMore
	}
	return learnMoreFallback
}
