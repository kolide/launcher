package localizationconsumer

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"strings"

	"github.com/kolide/launcher/v2/ee/agent/types"
)

//go:embed assets/*
var assets embed.FS

const (
	localizationsDataKey = "localization"
	defaultLocale        = "en-US"
)

type LocalizationConsumer struct {
	slogger          *slog.Logger
	store            types.KVStore
	localizationData types.LocalizationData
}

func NewLocalizationConsumer(slogger *slog.Logger, kvStore types.KVStore) (*LocalizationConsumer, error) {
	slogger = slogger.With("component", "localizationconsumer")
	t := &LocalizationConsumer{slogger: slogger, store: kvStore}
	t.localizationData.Translations = make(map[string]types.Translations)
	t.localizationData.Locale = defaultLocale

	// first load all the default translations form assets
	assetEntries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		return nil, fmt.Errorf("failed to read assets: %w", err)
	}

	for _, asset := range assetEntries {
		if asset.IsDir() || !strings.HasSuffix(asset.Name(), ".json") {
			continue
		}

		content, err := fs.ReadFile(assets, path.Join("assets", asset.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read asset %s: %w", asset.Name(), err)
		}

		// Each JSON file is { "localeKey": { "datetime": {...} } }
		var fileTranslations map[string]types.Translations
		if err := json.Unmarshal(content, &fileTranslations); err != nil {
			return nil, fmt.Errorf("failed to unmarshal asset %s: %w", asset.Name(), err)
		}

		maps.Copy(t.localizationData.Translations, fileTranslations)
	}

	// now load localization data from the store
	localizationDataRaw, err := t.store.Get([]byte(localizationsDataKey))
	if err != nil {
		return nil, fmt.Errorf("failed to get localization data from store: %w", err)
	}

	if len(localizationDataRaw) == 0 {
		return t, nil
	}

	var localizationFromStore types.LocalizationData
	if err := json.Unmarshal(localizationDataRaw, &localizationFromStore); err != nil {
		return nil, fmt.Errorf("failed to unmarshal localization data from store: %w", err)
	}

	t.mergeTranslations(localizationFromStore.Translations)

	// set the locale to the one from the store
	t.localizationData.Locale = localizationFromStore.Locale

	return t, nil
}

func (t *LocalizationConsumer) Update(data io.Reader) error {
	// parse the data into a types.LocalizationData
	var updatedLocalizationData types.LocalizationData
	if err := json.NewDecoder(data).Decode(&updatedLocalizationData); err != nil {
		return fmt.Errorf("failed to decode localization update: %w", err)
	}

	t.mergeTranslations(updatedLocalizationData.Translations)
	t.localizationData.Locale = updatedLocalizationData.Locale

	// marshal updated localization data into a byte slice
	updatedLocalizationDataBytes, err := json.Marshal(t.localizationData)
	if err != nil {
		return fmt.Errorf("failed to marshal localization update: %w", err)
	}

	// save to the store
	if err := t.store.Set([]byte(localizationsDataKey), updatedLocalizationDataBytes); err != nil {
		return fmt.Errorf("failed to save localization update to store: %w", err)
	}

	return nil
}

// mergeTranslations folds incoming per-locale translations into the current set,
// section by section. The server only sends the sections it owns (e.g. the
// notification "Learn More" label), so replacing a locale's whole entry would
// blank out the datetime translations that ship in the embedded assets. Empty
// incoming sections are left alone so the embedded values survive.
func (t *LocalizationConsumer) mergeTranslations(incoming map[string]types.Translations) {
	for locale, in := range incoming {
		merged := t.localizationData.Translations[locale]

		if in.Datetime != (types.Datetime{}) {
			merged.Datetime = in.Datetime
		}
		if in.Notifications != (types.Notifications{}) {
			merged.Notifications = in.Notifications
		}

		t.localizationData.Translations[locale] = merged
	}
}

func (t *LocalizationConsumer) LocalizationData() types.LocalizationData {
	return t.localizationData
}
