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
)

type Prefilter struct{ prg cel.Program }

func NewPrefilter(prefilter string) (*Prefilter, error) {
	env, err := cel.NewEnv(
		cel.Variable(celTopLevelVariable, cel.MapType(cel.StringType, cel.DynType)),
		cel.OptionalTypes(),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing CEL env: %w", err)
	}
	ast, iss := env.Compile(prefilter)
	if iss.Err() != nil {
		return nil, fmt.Errorf("compiling CEL prefilter: %w", iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("constructing program: %w", err)
	}

	return &Prefilter{prg: prg}, nil
}

func (p *Prefilter) Apply(obj any) (any, error) {
	out, _, err := p.prg.Eval(map[string]any{celTopLevelVariable: obj})
	if err != nil {
		return nil, fmt.Errorf("running prefilter: %w", err)
	}

	// If the output is empty, this object does not match the prefilter
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
