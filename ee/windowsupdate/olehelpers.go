package windowsupdate

import (
	"fmt"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// The helpers below replace the oleconv package. Each one calls
// oleutil.GetProperty (or CallMethod), extracts a typed Go value,
// and clears the VARIANT to release the underlying COM reference.
//
// For IDispatch results, the caller receives the raw *ole.IDispatch
// and is responsible for calling Release() when done. The VARIANT is
// NOT cleared in that case because ToIDispatch() is a pointer cast
// into the same memory -- clearing the VARIANT would release the
// IDispatch out from under the caller. See the arm64 panic note in
// ee/wmi/wmi.go for details.

func getPropertyString(disp *ole.IDispatch, property string, params ...interface{}) (string, error) {
	v, err := oleutil.GetProperty(disp, property, params...)
	if err != nil {
		return "", fmt.Errorf("getting property %s: %w", property, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("property %s: expected string, got %T", property, raw)
	}
	return s, nil
}

func getPropertyBool(disp *ole.IDispatch, property string) (bool, error) {
	v, err := oleutil.GetProperty(disp, property)
	if err != nil {
		return false, fmt.Errorf("getting property %s: %w", property, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return false, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("property %s: expected bool, got %T", property, raw)
	}
	return b, nil
}

func getPropertyInt32(disp *ole.IDispatch, property string) (int32, error) {
	v, err := oleutil.GetProperty(disp, property)
	if err != nil {
		return 0, fmt.Errorf("getting property %s: %w", property, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return 0, nil
	}
	i, ok := raw.(int32)
	if !ok {
		return 0, fmt.Errorf("property %s: expected int32, got %T", property, raw)
	}
	return i, nil
}

func getPropertyInt64(disp *ole.IDispatch, property string) (int64, error) {
	v, err := oleutil.GetProperty(disp, property)
	if err != nil {
		return 0, fmt.Errorf("getting property %s: %w", property, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return 0, nil
	}
	i, ok := raw.(int64)
	if !ok {
		return 0, fmt.Errorf("property %s: expected int64, got %T", property, raw)
	}
	return i, nil
}

func getPropertyUint32(disp *ole.IDispatch, property string) (uint32, error) {
	v, err := oleutil.GetProperty(disp, property)
	if err != nil {
		return 0, fmt.Errorf("getting property %s: %w", property, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return 0, nil
	}
	u, ok := raw.(uint32)
	if !ok {
		return 0, fmt.Errorf("property %s: expected uint32, got %T", property, raw)
	}
	return u, nil
}

func getPropertyTime(disp *ole.IDispatch, property string) (*time.Time, error) {
	v, err := oleutil.GetProperty(disp, property)
	if err != nil {
		return nil, fmt.Errorf("getting property %s: %w", property, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return nil, nil
	}
	t, ok := raw.(time.Time)
	if !ok {
		return nil, fmt.Errorf("property %s: expected time.Time, got %T", property, raw)
	}
	return &t, nil
}

// getPropertyDispatch returns the IDispatch inside a VARIANT property.
// The caller is responsible for calling Release() on the returned IDispatch.
// The VARIANT is NOT cleared here -- see package comment above.
func getPropertyDispatch(disp *ole.IDispatch, property string, params ...interface{}) (*ole.IDispatch, error) {
	v, err := oleutil.GetProperty(disp, property, params...)
	if err != nil {
		return nil, fmt.Errorf("getting property %s: %w", property, err)
	}

	raw := v.Value()
	if raw == nil {
		return nil, nil
	}

	return v.ToIDispatch(), nil
}

// callMethodInt32 calls a method and returns the int32 result.
func callMethodInt32(disp *ole.IDispatch, method string, params ...interface{}) (int32, error) {
	v, err := oleutil.CallMethod(disp, method, params...)
	if err != nil {
		return 0, fmt.Errorf("calling method %s: %w", method, err)
	}
	defer v.Clear()

	raw := v.Value()
	if raw == nil {
		return 0, nil
	}
	i, ok := raw.(int32)
	if !ok {
		return 0, fmt.Errorf("method %s: expected int32, got %T", method, raw)
	}
	return i, nil
}

// callMethodDispatch calls a method and returns the IDispatch result.
// The caller is responsible for calling Release() on the returned IDispatch.
func callMethodDispatch(disp *ole.IDispatch, method string, params ...interface{}) (*ole.IDispatch, error) {
	v, err := oleutil.CallMethod(disp, method, params...)
	if err != nil {
		return nil, fmt.Errorf("calling method %s: %w", method, err)
	}

	raw := v.Value()
	if raw == nil {
		return nil, nil
	}

	return v.ToIDispatch(), nil
}
