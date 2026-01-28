package translationsconsumer

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
)

//go:embed default_en-US.json
var defaultTranslationData string

const translationsDataKey = "translations"

type TranslationsConsumer struct {
	slogger *slog.Logger
	store   types.KVStore
	data    types.TranslationsData
}

func NewTranslationsConsumer(slogger *slog.Logger, kvStore types.KVStore) (*TranslationsConsumer, error) {
	slogger = slogger.With("component", "translations")
	translations := &TranslationsConsumer{slogger: slogger, store: kvStore}

	if err := translations.loadTranslations(); err != nil {
		// log it and return translations with default data
		slogger.Log(context.Background(), slog.LevelError,
			"error loading translations from store",
			"err", err,
		)
	}

	return translations, nil
}

func (t *TranslationsConsumer) Update(data io.Reader) error {
	// try to parse data
	var translationsData types.TranslationsData
	if err := json.NewDecoder(data).Decode(&translationsData); err != nil {
		t.slogger.Log(context.Background(), slog.LevelError,
			"error decoding translations data",
			"err", err,
		)
		return err
	}

	// if no error, save to store
	translationsDataBytes, err := json.Marshal(translationsData)
	if err != nil {
		t.slogger.Log(context.Background(), slog.LevelError,
			"error marshalling translations data",
			"err", err,
		)
		return err
	}

	if err := t.store.Set([]byte(translationsDataKey), translationsDataBytes); err != nil {
		t.slogger.Log(context.Background(), slog.LevelError,
			"error saving translations data to store",
			"err", err,
		)
		return err
	}

	// set data
	t.data = translationsData
	return nil
}

func (t *TranslationsConsumer) Translations() types.TranslationsData {
	return t.data
}

func (t *TranslationsConsumer) loadTranslations() error {
	// first load and set default translations
	defaultData := types.TranslationsData{}
	if err := json.Unmarshal([]byte(defaultTranslationData), &defaultData); err != nil {
		t.slogger.Log(context.Background(), slog.LevelError,
			"error unmarshalling default translations data",
			"err", err,
		)
		return err
	}

	t.data = defaultData

	// then load and set translations from the store
	translationsRaw, err := t.store.Get([]byte(translationsDataKey))
	if err != nil {
		t.slogger.Log(context.Background(), slog.LevelError,
			"error getting translations data from store",
			"err", err,
		)
		return err
	}

	if translationsRaw == nil {
		t.slogger.Log(context.Background(), slog.LevelDebug,
			"no translations data found in store, falling back to default",
		)
		return nil
	}

	var translationsFromStore types.TranslationsData
	if err := json.Unmarshal(translationsRaw, &translationsFromStore); err != nil {
		t.slogger.Log(context.Background(), slog.LevelError,
			"error unmarshalling translations data, falling back to default",
			"err", err,
		)
		return err
	}

	t.data = translationsFromStore
	return nil
}
