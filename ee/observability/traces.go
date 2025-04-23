package observability

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	instrumentationPkg        = "github.com/kolide/launcher/ee/observability"
	defaultSpanName           = "launcher/unknown"
	defaultAttributeNamespace = "unknown"
)

// StartHttpRequestSpan returns a copy of the request with a new context to include span info and span,
// including information about the calling function as appropriate.
// Standardizes the tracer name. The caller is always responsible for
// ending the span. `keyVals` should be a list of pairs, where the first in the pair is a
// string representing the attribute key and the second in the pair is the attribute value.
func StartHttpRequestSpan(r *http.Request, keyVals ...interface{}) (*http.Request, trace.Span) {
	ctx, span := startSpanWithExtractedAttributes(r.Context(), keyVals...)
	return r.WithContext(ctx), span
}

// StartSpan returns a new context and span, including information about the calling function
// as appropriate. Standardizes the tracer name. The caller is always responsible for
// ending the span. `keyVals` should be a list of pairs, where the first in the pair is a
// string representing the attribute key and the second in the pair is the attribute value.
func StartSpan(ctx context.Context, keyVals ...interface{}) (context.Context, trace.Span) {
	return startSpanWithExtractedAttributes(ctx, keyVals...)
}

// startSpanWithExtractedAttributes is the internal implementation of StartSpan and StartHttpRequestSpan
// with runtime.Caller(2) so that the caller of the wrapper function is used.
func startSpanWithExtractedAttributes(ctx context.Context, keyVals ...interface{}) (context.Context, trace.Span) {
	spanName := defaultSpanName

	opts := make([]trace.SpanStartOption, 0)

	// Extract information about the caller to set some standard attributes (code.filepath,
	// code.lineno, code.function) and to set more specific span and attribute names.
	// runtime.Caller(0) would return information about `startSpan` -- calling
	// runtime.Caller(1) will return information about the wrapper function calling `startSpan`.
	// runtime.Caller(2) will return information about the function calling the wrapper function
	programCounter, callerFile, callerLine, ok := runtime.Caller(2)
	if ok {
		opts = append(opts, trace.WithAttributes(
			semconv.CodeFilepath(callerFile),
			semconv.CodeLineNumber(callerLine),
		))

		// Extract the calling function name and use it to set code.function and the span name.
		if f := runtime.FuncForPC(programCounter); f != nil {
			spanName = filepath.Base(f.Name())
			opts = append(opts, trace.WithAttributes(semconv.CodeFunction(f.Name())))
		}
	}

	opts = append(opts, trace.WithAttributes(buildAttributes(callerFile, keyVals...)...))

	spanCtx, span := otel.Tracer(instrumentationPkg).Start(ctx, spanName, opts...)
	spanCtx = context.WithValue(spanCtx, multislogger.SpanIdKey, span.SpanContext().SpanID().String())
	spanCtx = context.WithValue(spanCtx, multislogger.TraceIdKey, span.SpanContext().TraceID().String())
	spanCtx = context.WithValue(spanCtx, multislogger.TraceSampledKey, span.SpanContext().IsSampled())

	return spanCtx, span
}

// SetError records the error on the span and sets the span's status to error.
func SetError(span trace.Span, err error) {
	// These are some otel ways to record errors. But we're not sure where they come through in GCP traces
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	// Dump the error into a span attribute, because :shrug:
	span.SetAttributes(semconv.ExceptionMessage(err.Error()))
}

// buildAttributes takes the given keyVals, expected to be pairs representing the key
// and value of each attribute, and parses them appropriately, ensuring that the keys
// have consistent and specific names. Pairs with invalid keys or values will be added
// as string attributes.
func buildAttributes(callerFile string, keyVals ...interface{}) []attribute.KeyValue {
	callerDir := defaultAttributeNamespace
	if callerFile != "" {
		// This is the closest we get to grabbing the package name from the caller -- e.g. if
		// the calling function is `/some/path/to/ee/localserver/request-controlservice.go`,
		// this will extract `localserver`.
		callerDir = filepath.Base(filepath.Dir(callerFile))
	}

	attrs := make([]attribute.KeyValue, 0)

	for i := 0; i < len(keyVals); i += 2 {
		// Keys must always be strings
		if _, ok := keyVals[i].(string); !ok {
			attrs = append(attrs, attribute.String(
				fmt.Sprintf("bad key type %T: %v", keyVals[i], keyVals[i]),
				fmt.Sprintf("%v", keyVals[i+1]),
			))
			continue
		}

		key := fmt.Sprintf("launcher.%s.%s", callerDir, keyVals[i])

		// Create an attribute of the appropriate type
		switch v := keyVals[i+1].(type) {
		case bool:
			attrs = append(attrs, attribute.Bool(key, v))
		case []bool:
			attrs = append(attrs, attribute.BoolSlice(key, v))
		case int:
			attrs = append(attrs, attribute.Int(key, v))
		case []int:
			attrs = append(attrs, attribute.IntSlice(key, v))
		case int64:
			attrs = append(attrs, attribute.Int64(key, v))
		case []int64:
			attrs = append(attrs, attribute.Int64Slice(key, v))
		case float64:
			attrs = append(attrs, attribute.Float64(key, v))
		case []float64:
			attrs = append(attrs, attribute.Float64Slice(key, v))
		case string:
			attrs = append(attrs, attribute.String(key, v))
		case []string:
			attrs = append(attrs, attribute.StringSlice(key, v))
		default:
			attrs = append(attrs, attribute.String(key, fmt.Sprintf("unsupported value of type %T: %v", keyVals[i+1], keyVals[i+1])))
		}
	}

	return attrs
}
