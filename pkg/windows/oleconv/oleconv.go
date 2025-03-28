// Package oleconv provides functions to convert from ole.VARIANT to
// expected types.
//
// It is originally from
// https://github.com/ceshihao/windowsupdate/blob/master/oleconv.go
package oleconv

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-ole/go-ole"
)

func okToErr(ok bool, t string) error {
	if !ok {
		return fmt.Errorf("not a %s", t)
	}
	return nil
}

func ToIDispatchErr(result *ole.VARIANT, err error) (*ole.IDispatch, error) {
	if err != nil {
		return nil, err
	}
	return result.ToIDispatch(), nil
}

func ToStringSliceErr(result *ole.VARIANT, err error) ([]string, error) {
	// It's not clear anything uses this. The know use cases are
	// better served by iStringCollectionToStringArrayErr
	if err != nil {
		return nil, err
	}
	array := result.ToArray()
	if array == nil {
		return nil, nil
	}
	return array.ToStringArray(), nil
}

func ToInt64Err(result *ole.VARIANT, err error) (int64, error) {
	if err != nil {
		return 0, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return 0, nil
	}

	value, ok := valueRaw.(int64)

	if err := result.Clear(); err != nil {
		return 0, errors.New("clearing int64")
	}

	return value, okToErr(ok, "int64")
}

func ToInt32Err(result *ole.VARIANT, err error) (int32, error) {
	if err != nil {
		return 0, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return 0, nil
	}
	value, ok := valueRaw.(int32)

	if err := result.Clear(); err != nil {
		return 0, errors.New("clearing int32")
	}

	return value, okToErr(ok, "int32")
}

func ToUint32Err(result *ole.VARIANT, err error) (uint32, error) {
	if err != nil {
		return 0, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return 0, nil
	}

	value, ok := valueRaw.(uint32)

	if err := result.Clear(); err != nil {
		return 0, errors.New("clearing uint32")
	}

	return value, okToErr(ok, "uint32")

}

func ToFloat64Err(result *ole.VARIANT, err error) (float64, error) {
	if err != nil {
		return 0, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return 0, nil
	}

	value, ok := valueRaw.(float64)

	if err := result.Clear(); err != nil {
		return 0, errors.New("clearing float64")
	}

	return value, okToErr(ok, "float64")
}

func ToFloat32Err(result *ole.VARIANT, err error) (float32, error) {
	if err != nil {
		return 0, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return 0, nil
	}

	value, ok := valueRaw.(float32)

	if err := result.Clear(); err != nil {
		return 0, errors.New("clearing float32")
	}

	return value, okToErr(ok, "float32")
}

func ToStringErr(result *ole.VARIANT, err error) (string, error) {
	if err != nil {
		return "", err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return "", nil
	}

	value, ok := valueRaw.(string)

	if err := result.Clear(); err != nil {
		return "", errors.New("clearing string")
	}

	return value, okToErr(ok, "string")
}

func ToBoolErr(result *ole.VARIANT, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return false, nil
	}

	value, ok := valueRaw.(bool)

	if err := result.Clear(); err != nil {
		return false, errors.New("clearing bool")
	}

	return value, okToErr(ok, "bool")
}

func ToTimeErr(result *ole.VARIANT, err error) (*time.Time, error) {
	if err != nil {
		return nil, err
	}

	valueRaw := result.Value()

	if valueRaw == nil {
		return nil, nil
	}

	value, ok := valueRaw.(time.Time)

	if err := result.Clear(); err != nil {
		return nil, errors.New("clearing time")
	}

	return &value, okToErr(ok, "time")
}
