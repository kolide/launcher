package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/stretchr/testify/require"
)

// writeLocalizationFile is a tiny test helper that writes a LocalizationData
// to a temp file and returns the path.
func writeTestLocalizationFile(t *testing.T, ld types.LocalizationData) string {
	t.Helper()
	data, err := json.Marshal(ld)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "localization.json")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

// localizationDataWithLearnMore builds a LocalizationData with the given
// locale and Learn More translation.
func localizationDataWithLearnMore(locale, learnMore string) types.LocalizationData {
	var n types.Notifications
	n.Actions.LearnMore = learnMore

	return types.LocalizationData{
		Locale: locale,
		Translations: map[string]types.Translations{
			locale: {Notifications: n},
		},
	}
}

func TestLoadLocalizationData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        func(t *testing.T) string
		wantLocale  string
		wantInMap   bool
		wantMissing bool
	}{
		{
			name:        "empty path returns zero value",
			path:        func(t *testing.T) string { return "" },
			wantMissing: true,
		},
		{
			name: "missing file returns zero value",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist.json")
			},
			wantMissing: true,
		},
		{
			name: "corrupt JSON returns zero value",
			path: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "bad.json")
				require.NoError(t, os.WriteFile(p, []byte("not valid json"), 0644))
				return p
			},
			wantMissing: true,
		},
		{
			name: "valid file is parsed",
			path: func(t *testing.T) string {
				return writeTestLocalizationFile(t, localizationDataWithLearnMore("es-ES", "Más información"))
			},
			wantLocale: "es-ES",
			wantInMap:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ld := loadLocalizationData(tt.path(t))

			if tt.wantMissing {
				require.Empty(t, ld.Locale)
				require.Empty(t, ld.Translations)
				return
			}

			require.Equal(t, tt.wantLocale, ld.Locale)
			if tt.wantInMap {
				require.Contains(t, ld.Translations, tt.wantLocale)
			}
		})
	}
}

func TestLearnMoreLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "empty path falls back to English",
			path: func(t *testing.T) string { return "" },
			want: learnMoreFallback,
		},
		{
			name: "missing file falls back to English",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing.json")
			},
			want: learnMoreFallback,
		},
		{
			name: "corrupt JSON falls back to English",
			path: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "bad.json")
				require.NoError(t, os.WriteFile(p, []byte("{not json"), 0644))
				return p
			},
			want: learnMoreFallback,
		},
		{
			name: "active locale not in translations falls back to English",
			path: func(t *testing.T) string {
				ld := types.LocalizationData{
					Locale: "fr-FR",
					Translations: map[string]types.Translations{
						"es-ES": {},
					},
				}
				return writeTestLocalizationFile(t, ld)
			},
			want: learnMoreFallback,
		},
		{
			name: "locale present but learn_more empty falls back to English",
			path: func(t *testing.T) string {
				return writeTestLocalizationFile(t, localizationDataWithLearnMore("es-ES", ""))
			},
			want: learnMoreFallback,
		},
		{
			name: "spanish translation returns localized value",
			path: func(t *testing.T) string {
				return writeTestLocalizationFile(t, localizationDataWithLearnMore("es-ES", "Más información"))
			},
			want: "Más información",
		},
		{
			name: "japanese translation returns localized value",
			path: func(t *testing.T) string {
				return writeTestLocalizationFile(t, localizationDataWithLearnMore("ja", "詳細"))
			},
			want: "詳細",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, learnMoreLabel(tt.path(t)))
		})
	}
}
