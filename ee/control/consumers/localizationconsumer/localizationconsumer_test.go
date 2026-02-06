package localizationconsumer

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/kolide/launcher/ee/agent/storage/inmemory"

	"github.com/stretchr/testify/require"
)

func TestLocalizationConsumer(t *testing.T) {

	const startingTestLocale = "starting-locale"
	const endingTestLocale = "ending-locale"

	t.Parallel()

	slogger := slog.Default()

	tests := []struct {
		name string

		seedStoreData          []byte
		expectedStartingLocale string

		updateData           []byte
		expectedEndingLocale string
	}{
		{
			name:                   "happy path - no store data - no update data",
			expectedStartingLocale: defaultLocale,
		},
		{
			name:                   "happy path - with store data - no update data",
			expectedStartingLocale: startingTestLocale,
			seedStoreData:          fmt.Appendf(nil, `{"locale":"%s","translations":{"%s":{"datetime":{"distance_in_words":{"less_than_x_minutes":{"one":"%s","other":"%s"}}}}}}`, startingTestLocale, startingTestLocale, startingTestLocale, startingTestLocale),
		},
		{
			name:                   "happy path - with store data - with update data",
			expectedStartingLocale: startingTestLocale,
			seedStoreData:          fmt.Appendf(nil, `{"locale":"%s","translations":{"%s":{"datetime":{"distance_in_words":{"less_than_x_minutes":{"one":"%s","other":"%s"}}}}}}`, startingTestLocale, startingTestLocale, startingTestLocale, startingTestLocale),
			updateData:             fmt.Appendf(nil, `{"locale":"%s","translations":{"%s":{"datetime":{"distance_in_words":{"less_than_x_minutes":{"one":"%s","other":"%s"}}}}}}`, endingTestLocale, endingTestLocale, endingTestLocale, endingTestLocale),
			expectedEndingLocale:   endingTestLocale,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := inmemory.NewStore()

			if len(tt.seedStoreData) > 0 {
				store.Set([]byte(localizationsDataKey), tt.seedStoreData)
			}

			consumer, err := NewLocalizationConsumer(slogger, store)
			require.NoError(t, err)
			require.NotNil(t, consumer)
			require.Equal(t, tt.expectedStartingLocale, consumer.localizationData.Locale)

			if len(tt.seedStoreData) <= 0 {
				// with no store data, the default locale should be used
				require.Equal(t, "less than a minute", consumer.localizationData.Translations[consumer.localizationData.Locale].Datetime.DistanceInWords.LessThanXMinutes.One)
				return
			}

			// with store data, the store data should be used
			require.Equal(t, startingTestLocale, consumer.localizationData.Translations[consumer.localizationData.Locale].Datetime.DistanceInWords.LessThanXMinutes.One)

			if len(tt.updateData) <= 0 {
				return
			}

			require.NoError(t, consumer.Update(bytes.NewReader(tt.updateData)))
			require.Equal(t, tt.expectedEndingLocale, consumer.localizationData.Locale)
			require.Equal(t, endingTestLocale, consumer.localizationData.Translations[consumer.localizationData.Locale].Datetime.DistanceInWords.LessThanXMinutes.One)

			// create a new consumer to verify that updates are persisted
			consumer, err = NewLocalizationConsumer(slogger, store)
			require.NoError(t, err)
			require.NotNil(t, consumer)
			require.Equal(t, tt.expectedEndingLocale, consumer.localizationData.Locale)
			require.Equal(t, endingTestLocale, consumer.localizationData.Translations[consumer.localizationData.Locale].Datetime.DistanceInWords.LessThanXMinutes.One)
		})
	}
}
