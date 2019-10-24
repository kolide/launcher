package dataflatten

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Row struct {
	Path  []string
	Value string
}

const defaultPathSeperator = "/"

type Flattener struct {
	keyForArraysOfMaps string
	includeNils        bool
	arrayKeyName       string
	rows               []Row
}

type FlattenOpts func(*Flattener)

func IncludeNulls() FlattenOpts {
	return func(fl *Flattener) {
		fl.includeNils = true
	}
}

func ArrayKeyName(s string) FlattenOpts {
	return func(fl *Flattener) {
		fl.arrayKeyName = s
	}
}

// TODO: Write this better
// Note that this returns an array with an unstable order.
func Flatten(data interface{}, opts ...FlattenOpts) ([]Row, error) {
	fl := &Flattener{
		rows: []Row{},
	}

	for _, opt := range opts {
		opt(fl)
	}

	if err := fl.descend([]string{}, data); err != nil {
		return nil, err
	}

	return fl.rows, nil
}

func (fl *Flattener) descend(path []string, data interface{}) error {
	switch v := data.(type) {
	case []interface{}:
		for i, e := range v {
			if err := fl.descend(append(path, strconv.Itoa(i)), e); err != nil {
				return errors.Wrap(err, "flattening")
			}
		}
		return nil
	case map[string]interface{}:
		for k, e := range v {
			if err := fl.descend(append(path, k), e); err != nil {
				return errors.Wrap(err, "flattening")
			}
		}
		return nil
	case nil:
		if fl.includeNils {
			fl.rows = append(fl.rows, Row{Path: path, Value: ""})
		}
		return nil

	default:
		// non-iterable. stringify and be done
		stringValue, err := stringify(v)
		if err != nil {
			return errors.Wrapf(err, "flattening at path %v", path)
		}
		fl.rows = append(fl.rows, Row{Path: path, Value: stringValue})
		return nil

	}
	return nil

}

func stringify(data interface{}) (string, error) {
	switch v := data.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case float64:
		// using fmt is a shortcut around a bunch of ugly
		// numeric parsing. json returns float64 for
		// ~everything.
		return fmt.Sprintf("%v", v), nil
	case int:
		return strconv.Itoa(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return "", errors.Errorf("unknown type on %v", v)
	}
}

func (r Row) StringPath() string {
	return strings.Join(r.Path, defaultPathSeperator)
}

func (r Row) ParentKey() (string, string) {
	switch len(r.Path) {
	case 0:
		return "", ""
	case 1:
		return "", r.Path[0]
	}

	return strings.Join(r.Path[:len(r.Path)-1], defaultPathSeperator), r.Path[len(r.Path)-1]
}
