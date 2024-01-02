package flareconsumer

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	knapsackMock "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/control/consumers/flareconsumer/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFlareConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		flarer       func(t *testing.T) flarer
		errAssertion require.ErrorAssertionFunc
	}{
		{
			name: "happy path",
			flarer: func(t *testing.T) flarer {
				flarer := mocks.NewFlarer(t)
				flarer.On("RunFlare", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
				return flarer
			},
			errAssertion: require.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSack := knapsackMock.NewKnapsack(t)
			mockSack.On("Slogger").Return(slog.New(slog.NewJSONHandler(io.Discard, nil))).Maybe()
			f := New(mockSack)
			f.flarer = tt.flarer(t)
			f.newFlareStream = func(note, uploadRequestURL string) (io.WriteCloser, error) {
				// whatever, it implements write closer
				return &io.PipeWriter{}, nil
			}

			tt.errAssertion(t, f.Do(bytes.NewBuffer([]byte(`{"upload_url":"https://example.com"}`))))
		})
	}
}
