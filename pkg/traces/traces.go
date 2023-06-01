package traces

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	applicationName = "launcher"
	defaultSpanName = "launcher/unknown"
)

// New returns a new context and span, including information about the calling function
// as appropriate. Standardizes the tracer name. The caller is always responsible for
// ending the span.
func New(ctx context.Context, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	spanName := defaultSpanName

	pc, file, line, ok := runtime.Caller(1)
	if ok {
		opts = append(opts, trace.WithAttributes(
			attribute.String("code.filepath", file),
			attribute.Int("code.lineno", line),
		))

		if f := runtime.FuncForPC(pc); f != nil {
			spanName = filepath.Base(f.Name())
			opts = append(opts, trace.WithAttributes(attribute.String("code.function", f.Name())))
		}
	}

	return otel.Tracer(applicationName).Start(ctx, spanName, opts...)
}

// AttributeName returns a standardized, namespaced, and appropriately specific name for the
// given attribute, in the format `launcher.<pkg>.<attr>`, e.g. launcher.tablehelpers.args.
func AttributeName(packageName string, baseAttrName string) string {
	return fmt.Sprintf("%s.%s.%s", applicationName, packageName, baseAttrName)
}
