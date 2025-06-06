package bufspanprocessor

import (
	"context"
	"log/slog"
	"sync"

	"github.com/kolide/launcher/pkg/log/multislogger"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type BufSpanProcessor struct {
	slogger       *slog.Logger
	bufferedSpans []sdktrace.ReadOnlySpan
	bufMu         *sync.Mutex

	childProcessor   sdktrace.SpanProcessor
	MaxBufferedSpans int
}

func NewBufSpanProcessor(maxBufferedSpans int) *BufSpanProcessor {
	return &BufSpanProcessor{
		slogger:          multislogger.NewNopLogger(),
		bufferedSpans:    make([]sdktrace.ReadOnlySpan, 0),
		bufMu:            &sync.Mutex{},
		MaxBufferedSpans: maxBufferedSpans,
	}
}

// HasProcessor returns true if the processor has been set.
func (b *BufSpanProcessor) HasProcessor() bool {
	b.bufMu.Lock()
	defer b.bufMu.Unlock()

	return b.childProcessor != nil
}

func (b *BufSpanProcessor) SetSlogger(slogger *slog.Logger) {
	b.slogger = slogger.With("component", "buf_span_processor")
}

// SetChildProcessor sets the processor that will receive the spans.
// Any buffered spans will be sent to the processor.
// After the child processor is set, spans will no longer be buffered
// or have attributes added, they will simply be passed straight to the
// child processor.
// If a processor was already set, it will be shutdown.
func (b *BufSpanProcessor) SetChildProcessor(p sdktrace.SpanProcessor) {
	if b.childProcessor != nil {
		if err := b.childProcessor.Shutdown(context.Background()); err != nil {
			b.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not shut down previous child processor before setting new one",
				"err", err,
			)
		}
	}

	b.childProcessor = p

	// send the spans that were buffered before the processor was set
	b.bufMu.Lock()
	defer b.bufMu.Unlock()
	for i, span := range b.bufferedSpans {
		// We've seen a panic in b.childProcessor.OnEnd(span), where it ultimately calls
		// span.SpanContext().IsSampled() inside enqueueDrop. It's not really clear exactly
		// how the span could be nil, but it appears to be the only possibility here.
		if span == nil {
			b.slogger.Log(context.TODO(), slog.LevelInfo,
				"got nil span",
				"buffered_span_idx", i,
			)
			continue
		}
		b.childProcessor.OnEnd(span)
	}

	b.slogger.Log(context.TODO(), slog.LevelDebug,
		"forwarded all buffered spans to child processor",
		"buffer_length", len(b.bufferedSpans),
	)

	// now that the spans are sent, clear the buffer
	b.bufferedSpans = nil
}

// OnStart is called when a span is started. It is called synchronously
// and should not block. OnStart will append the configured attributes.
func (b *BufSpanProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	if b.childProcessor == nil {
		return
	}

	b.childProcessor.OnStart(parent, s)
}

// OnEnd is called when span is finished. It is called synchronously and
// hence not block.
func (b *BufSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	if b.childProcessor != nil {
		b.childProcessor.OnEnd(s)
		return
	}

	b.bufMu.Lock()
	defer b.bufMu.Unlock()

	if len(b.bufferedSpans) >= b.MaxBufferedSpans {
		return
	}

	b.bufferedSpans = append(b.bufferedSpans, s)
}

// Shutdown is called when the SDK shuts down. Any cleanup or release of
// resources held by the processor should be done in this call.
//
// Calls to OnStart, OnEnd, or ForceFlush after this has been called
// should be ignored.
//
// All timeouts and cancellations contained in ctx must be honored, this
// should not block indefinitely.
func (b *BufSpanProcessor) Shutdown(ctx context.Context) error {
	if b.childProcessor == nil {
		return nil
	}

	return b.childProcessor.Shutdown(ctx)
}

// ForceFlush exports all ended spans to the configured Exporter that have not yet
// been exported.  It should only be called when absolutely necessary, such as when
// using a FaaS provider that may suspend the process after an invocation, but before
// the Processor can export the completed spans.
func (b *BufSpanProcessor) ForceFlush(ctx context.Context) error {
	if b.childProcessor == nil {
		return nil
	}

	return b.childProcessor.ForceFlush(ctx)
}
