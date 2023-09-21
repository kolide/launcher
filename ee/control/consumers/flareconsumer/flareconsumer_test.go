package flareconsumer

import (
	"bytes"
	"testing"

	"github.com/kolide/launcher/ee/control/consumers/flareconsumer/mocks"
	knapsackMock "github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFlareConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		flarer       func(t *testing.T) flarer
		shipper      func(t *testing.T) shipper
		errAssertion require.ErrorAssertionFunc
	}{
		{
			name: "happy path",
			flarer: func(t *testing.T) flarer {
				flarer := mocks.NewFlarer(t)
				flarer.On("RunFlare", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
				return flarer
			},
			shipper: func(t *testing.T) shipper {
				shipper := mocks.NewShipper(t)
				shipper.On("Ship", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
				return shipper
			},
			errAssertion: require.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSack := knapsackMock.NewKnapsack(t)

			f := New(mockSack, tt.flarer(t), tt.shipper(t))

			tt.errAssertion(t, f.Do(bytes.NewBuffer([]byte(`{"note":"the_note"}`))))
		})
	}
}
