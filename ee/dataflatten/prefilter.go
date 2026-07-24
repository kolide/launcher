package dataflatten

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/traits"
)

const (
	// celTopLevelVariable is our fixed CEL variable name; we bind each object to this variable name
	// when evaluating the prefilter. CEL requires every variable to be declared before parsing;
	// it is simpler for us to require the variable name to be `this` than to attempt to parse the variable
	// name out of the prefilter provided in the query.
	celTopLevelVariable = "this"
	// celCostLimit is our threshold for maximum CPU usage for a single evaluation of a prefilter expression.
	// The unit is the number of operations performed during evaluation; 10 million should be a comfortably high number.
	celCostLimit = 10000000
)

type Prefilter struct{ prg cel.Program }

func NewPrefilter(prefilter string) (*Prefilter, error) {
	env, err := cel.NewEnv(
		cel.Variable(celTopLevelVariable, cel.DynType),
		cel.OptionalTypes(),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing CEL env: %w", err)
	}
	ast, iss := env.Compile(prefilter)
	if iss.Err() != nil {
		return nil, fmt.Errorf("compiling CEL prefilter: %w", iss.Err())
	}
	prg, err := env.Program(ast, cel.CostLimit(celCostLimit))
	if err != nil {
		return nil, fmt.Errorf("constructing program: %w", err)
	}

	return &Prefilter{prg: prg}, nil
}

// Apply runs the prefilter on the given object. It will return nil if the
// object does not match the filter; otherwise, it will return a transformed
// object with only the selected fields.
func (p *Prefilter) Apply(obj any) (any, error) {
	out, _, err := p.prg.Eval(map[string]any{celTopLevelVariable: obj})
	if err != nil {
		return nil, fmt.Errorf("running prefilter: %w", err)
	}

	// If the output is empty, this object does not match the prefilter
	if sz, ok := out.(traits.Sizer); ok && sz.Size() == types.IntZero {
		return nil, nil
	}

	native, err := out.ConvertToNative(reflect.TypeFor[any]())
	if err != nil {
		return nil, fmt.Errorf("converting prefilter result: %w", err)
	}

	return normalizeStringKeys(native), nil
}

// normalizeStringKeys recursively rewrites map[any]any (as produced by CEL's
// ConvertToNative) to map[string]any so dataflatten can descend into it.
func normalizeStringKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeStringKeys(val)
		}
		return t
	case map[any]any:
		converted := make(map[string]any, len(t))
		for k, val := range t {
			converted[fmt.Sprintf("%v", k)] = normalizeStringKeys(val)
		}
		return converted
	case []any:
		for i, val := range t {
			t[i] = normalizeStringKeys(val)
		}
		return t
	default:
		return v
	}
}
