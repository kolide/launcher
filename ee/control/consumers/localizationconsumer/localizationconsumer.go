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

	"github.com/kolide/launcher/ee/agent/types"
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
		return nil, err
	}

	for _, asset := range assetEntries {
		if asset.IsDir() || !strings.HasSuffix(asset.Name(), ".json") {
			continue
		}

		content, err := fs.ReadFile(assets, path.Join("assets", asset.Name()))
		if err != nil {
			return nil, err
		}

		// Each JSON file is { "localeKey": { "datetime": {...} } }
		var fileTranslations map[string]types.Translations
		if err := json.Unmarshal(content, &fileTranslations); err != nil {
			return nil, err
		}

		maps.Copy(t.localizationData.Translations, fileTranslations)
	}

	// now load localization data from the store
	localizationDataRaw, err := t.store.Get([]byte(localizationsDataKey))
	if err != nil {
		return nil, err
	}

	if len(localizationDataRaw) == 0 {
		return t, nil
	}

	var localizationFromStore types.LocalizationData
	if err := json.Unmarshal(localizationDataRaw, &localizationFromStore); err != nil {
		return nil, err
	}

	maps.Copy(t.localizationData.Translations, localizationFromStore.Translations)

	// set the locale to the one from the store
	t.localizationData.Locale = localizationFromStore.Locale

	return t, nil
}

func (t *LocalizationConsumer) Update(data io.Reader) error {
	// parse the data into a types.LocalizationData
	var updatedLocalizationData types.LocalizationData
	if err := json.NewDecoder(data).Decode(&updatedLocalizationData); err != nil {
		return fmt.Errorf("failed to decode localization json: %w", err)
	}

	maps.Copy(t.localizationData.Translations, updatedLocalizationData.Translations)
	t.localizationData.Locale = updatedLocalizationData.Locale

	// marshal updated localization data into a byte slice
	updatedLocalizationDataBytes, err := json.Marshal(t.localizationData)
	if err != nil {
		return fmt.Errorf("failed to marshal localization data: %w", err)
	}

	// save to the store
	if err := t.store.Set([]byte(localizationsDataKey), updatedLocalizationDataBytes); err != nil {
		return fmt.Errorf("failed to save localization to store: %w", err)
	}

	return nil
}

func (t *LocalizationConsumer) LocalizationData() types.LocalizationData {
	return t.localizationData
}
