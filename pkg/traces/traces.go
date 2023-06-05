package traces

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	ApplicationName           = "launcher"
	defaultSpanName           = "launcher/unknown"
	defaultAttributeNamespace = "unknown"
)

// StartSpan returns a new context and span, including information about the calling function
// as appropriate. Standardizes the tracer name. The caller is always responsible for
// ending the span. `keyVals` should be a list of pairs, where the first in the pair is a
// string representing the attribute key and the second in the pair is the attribute value.
func StartSpan(ctx context.Context, keyVals ...interface{}) (context.Context, trace.Span) {
	spanName := defaultSpanName

	opts := make([]trace.SpanStartOption, 0)

	// Extract information about the caller to set some standard attributes (code.filepath,
	// code.lineno, code.function) and to set more specific span and attribute names.
	// runtime.Caller(0) would return information about `StartSpan` -- calling
	// runtime.Caller(1) will return information about the function calling `StartSpan`.
	pc, callerFile, callerLine, ok := runtime.Caller(1)
	if ok {
		opts = append(opts, trace.WithAttributes(
			semconv.CodeFilepath(callerFile),
			semconv.CodeLineNumber(callerLine),
		))

		// Extract the calling function name and use it to set code.function and the span name.
		if f := runtime.FuncForPC(pc); f != nil {
			spanName = filepath.Base(f.Name())
			opts = append(opts, trace.WithAttributes(semconv.CodeFunction(f.Name())))
		}
	}

	opts = append(opts, trace.WithAttributes(buildAttributes(callerFile, keyVals...)...))

	return otel.Tracer(ApplicationName).Start(ctx, spanName, opts...)
}

// buildAttributes takes the given keyVals, expected to be pairs representing the key
// and value of each attribute, and parses them appropriately, ensuring that the keys
// have consistent and specific names. Pairs that cannot be parsed will be discarded.
func buildAttributes(callerFile string, keyVals ...interface{}) []attribute.KeyValue {
	callerDir := defaultAttributeNamespace
	if callerFile != "" {
		callerDir = filepath.Base(filepath.Dir(callerFile))
	}

	attrs := make([]attribute.KeyValue, 0)

	for i := 0; i < len(keyVals); i += 2 {
		// Keys must always be strings
		if reflect.TypeOf(keyVals[i]).String() != "string" {
			continue
		}
		key := fmt.Sprintf("%s.%s.%s", ApplicationName, callerDir, keyVals[i])

		// Create an attribute of the appropriate type, dropping ones we cannot determine
		// or do not support
		switch keyVals[i+1].(type) {
		case bool:
			attrs = append(attrs, attribute.Bool(key, keyVals[i+1].(bool)))
		case []bool:
			attrs = append(attrs, attribute.BoolSlice(key, keyVals[i+1].([]bool)))
		case int:
			attrs = append(attrs, attribute.Int(key, keyVals[i+1].(int)))
		case []int:
			attrs = append(attrs, attribute.IntSlice(key, keyVals[i+1].([]int)))
		case int64:
			attrs = append(attrs, attribute.Int64(key, keyVals[i+1].(int64)))
		case []int64:
			attrs = append(attrs, attribute.Int64Slice(key, keyVals[i+1].([]int64)))
		case float64:
			attrs = append(attrs, attribute.Float64(key, keyVals[i+1].(float64)))
		case []float64:
			attrs = append(attrs, attribute.Float64Slice(key, keyVals[i+1].([]float64)))
		case string:
			attrs = append(attrs, attribute.String(key, keyVals[i+1].(string)))
		case []string:
			attrs = append(attrs, attribute.StringSlice(key, keyVals[i+1].([]string)))
		}
	}

	return attrs
}
