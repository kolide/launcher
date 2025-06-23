package bufspanprocessor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/observability/bufspanprocessor/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestBufSpanProcessor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			maxBufSpans := 10

			// create the buf span processor
			bsp := NewBufSpanProcessor(maxBufSpans)

			// callling these for shameless code coverage
			require.NoError(t, bsp.ForceFlush(context.TODO()))
			require.NoError(t, bsp.Shutdown(context.TODO()))

			require.False(t, bsp.HasProcessor(), "should not have a processor")

			// set global trace provider
			otel.SetTracerProvider(sdktrace.NewTracerProvider(
				sdktrace.WithSpanProcessor(bsp),
			))

			// don't exceed max spans
			createSpans(maxBufSpans * 2)
			require.Equal(t, maxBufSpans, len(bsp.bufferedSpans), "should not exceed max spans")

			firstChildProcessor := mocks.NewSpanProcessor(t)
			firstChildProcessor.On("OnEnd", mock.Anything).Return().Times(maxBufSpans)

			bsp.SetChildProcessor(firstChildProcessor)
			// require.True(t, firstChildProcessor.onEndCalled, "should have called OnEnd")
			require.Nil(t, bsp.bufferedSpans, "should have cleared buffered spans")

			// wait for the spans to be transferred to child processor
			for len(bsp.bufferedSpans) > 0 {
				time.Sleep(100 * time.Millisecond)
			}

			firstChildProcessor.On("Shutdown", mock.Anything).Return(nil).Once()

			// make a new child processor
			secondChildProcessor := mocks.NewSpanProcessor(t)
			bsp.SetChildProcessor(secondChildProcessor)

			// now all the spans should go straight to the new child processor (no buffering)
			secondChildProcessor.On("OnStart", mock.Anything, mock.Anything).Return().Times(maxBufSpans * 2)
			secondChildProcessor.On("OnEnd", mock.Anything).Return().Times(maxBufSpans * 2)
			createSpans(maxBufSpans * 2)

			secondChildProcessor.On("ForceFlush", mock.Anything).Return(nil).Once()
			secondChildProcessor.On("Shutdown", mock.Anything).Return(nil).Once()

			require.NoError(t, bsp.ForceFlush(context.TODO()))
			require.NoError(t, bsp.Shutdown(context.TODO()))
		})
	}
}

func createSpans(count int) {
	wg := &sync.WaitGroup{}
	for i := 0; i < count; i++ {
		wg.Add(1)

		go func() {
			_, span := otel.Tracer("test_tracer").Start(context.Background(), "test")
			span.End()
			wg.Done()
		}()
	}
	wg.Wait()
}
