package dataflatten

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/traits"
)

const (
	// hardcodedCELPrefilter is hardcoded for ease of iteration, but would be passed in with other FlattenOpts,
	// ultimately coming from the query context.
	hardcodedCELPrefilter = `record.type == "user" ? {
  ?"sourceToolAssistantUUID": record.?sourceToolAssistantUUID,
  ?"entrypoint":             record.?entrypoint,
  ?"timestamp":              record.?timestamp,
  ?"cwd":                    record.?cwd
} : {}`
	celTopLevelVariable = "record"
)

func NewCELPrefilter(prefilter string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Variable(celTopLevelVariable, cel.MapType(cel.StringType, cel.DynType)),
		cel.OptionalTypes(),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing CEL env: %w", err)
	}
	ast, iss := env.Parse(prefilter)
	if iss.Err() != nil {
		return nil, fmt.Errorf("parsing CEL prefilter: %w", iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("constructing program: %w", err)
	}

	return prg, nil
}

func RunCELPrefilter(prg cel.Program, data any) (any, error) {
	out, _, err := prg.Eval(map[string]any{celTopLevelVariable: data})
	if err != nil {
		return nil, fmt.Errorf("running prefilter: %w", err)
	}
	// Check for and discard empty (i.e. filtered) records now
	if sz, ok := out.(traits.Sizer); ok && sz.Size() == types.IntZero {
		return nil, nil
	}

	// Convert back to Go type so that dataflatten can handle it later
	native, err := out.ConvertToNative(reflect.TypeFor[map[string]any]())
	if err != nil {
		return nil, fmt.Errorf("converting prefilter result: %w", err)
	}

	m, ok := native.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("prefilter result was not a map, got %T", native)
	}

	return m, nil
}
