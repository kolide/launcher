package translationsconsumer

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTranslationsConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupStore     func(*testing.T) types.KVStore
		expectedLocale string
	}{
		{
			name: "loads default translations when store is empty",
			setupStore: func(t *testing.T) types.KVStore {
				return inmemory.NewStore()
			},
			expectedLocale: "en-US",
		},
		{
			name: "loads translations from store",
			setupStore: func(t *testing.T) types.KVStore {
				store := inmemory.NewStore()
				translationsData := types.TranslationsData{
					Locale: "fr-FR",
				}
				data, err := json.Marshal(translationsData)
				require.NoError(t, err)
				err = store.Set([]byte(translationsDataKey), data)
				require.NoError(t, err)
				return store
			},
			expectedLocale: "fr-FR",
		},
		{
			name: "handles bad JSON in store - falls back to defaults",
			setupStore: func(t *testing.T) types.KVStore {
				store := inmemory.NewStore()
				err := store.Set([]byte(translationsDataKey), []byte("invalid json"))
				require.NoError(t, err)
				return store
			},
			expectedLocale: "en-US",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			slogger := slog.New(slog.DiscardHandler)
			store := tt.setupStore(t)

			consumer, err := NewTranslationsConsumer(slogger, store)
			require.NoError(t, err)
			require.NotNil(t, consumer)

			translations := consumer.Translations()
			assert.Equal(t, tt.expectedLocale, translations.Locale)
		})
	}
}

func TestTranslationsConsumer_Updates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		updateLocale   string
		expectedLocale string
		wantErr        bool
	}{
		{
			name:           "handles happy path",
			updateLocale:   `{"locale": "de-DE", "translations": {}}`,
			expectedLocale: "de-DE",
			wantErr:        false,
		},
		{
			name:           "handles bad JSON",
			updateLocale:   `invalid json`,
			expectedLocale: "en-US",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := inmemory.NewStore()

			consumer, err := NewTranslationsConsumer(slog.New(slog.DiscardHandler), store)
			require.NoError(t, err)

			err = consumer.Update(bytes.NewReader([]byte(tt.updateLocale)))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.expectedLocale, consumer.Translations().Locale)

			// ensure expected data was saved to store
			consumer, err = NewTranslationsConsumer(slog.New(slog.DiscardHandler), store)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedLocale, consumer.Translations().Locale)
		})
	}
}
