package dataflattentable

import "github.com/kolide/launcher/pkg/dataflatten"

// funcFlattener is a wraper over a flattening func to convert it to a flattener interface. It should be folded
// into pkg/dataflatten sometime
type funcFlattener struct {
	fn func([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
}

func flattenerFromFunc(fn func([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)) funcFlattener {
	return funcFlattener{fn: fn}
}

func (fnf funcFlattener) FlattenBytes(raw []byte, flattenOpts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	return fnf.fn(raw, flattenOpts...)
}
