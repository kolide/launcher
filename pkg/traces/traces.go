package traces

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	ApplicationName = "launcher"
	defaultSpanName = "launcher/unknown"
)

// StartSpan returns a new context and span, including information about the calling function
// as appropriate. Standardizes the tracer name. The caller is always responsible for
// ending the span.
func StartSpan(ctx context.Context, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	spanName := defaultSpanName

	opts := make([]trace.SpanStartOption, 0)

	// Extract information about the caller to set some standard attributes (code.filepath,
	// code.lineno, code.function) and to set more specific span and attribute names.
	// runtime.Caller(0) would return information about `StartSpan` -- calling
	// runtime.Caller(1) will return information about the function calling `StartSpan`.
	pc, file, line, ok := runtime.Caller(1)
	if ok {
		opts = append(opts, trace.WithAttributes(
			semconv.CodeFilepath(file),
			semconv.CodeLineNumber(line),
		))

		// Prepend directory name to attribute keys for specificity.
		callerDir := filepath.Base(filepath.Dir(file))
		for _, attr := range attrs {
			attr.Key = attribute.Key(fmt.Sprintf("%s.%s.%s", ApplicationName, callerDir, attr.Key))
			opts = append(opts, trace.WithAttributes(attr))
		}

		// Extract the calling function name and use it to set code.function and the span name.
		if f := runtime.FuncForPC(pc); f != nil {
			spanName = filepath.Base(f.Name())
			opts = append(opts, trace.WithAttributes(semconv.CodeFunction(f.Name())))
		}
	} else {
		// Could not extract information about caller -- include attributes as they are.
		for _, attr := range attrs {
			opts = append(opts, trace.WithAttributes(attr))
		}
	}

	return otel.Tracer(ApplicationName).Start(ctx, spanName, opts...)
}
