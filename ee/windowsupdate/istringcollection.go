package windowsupdate

import (
	"fmt"

	"github.com/go-ole/go-ole"
)

// iStringCollectionToStringArray takes an IDispatch to a string collection
// and returns the array of strings.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-istringcollection
func iStringCollectionToStringArray(disp *ole.IDispatch) ([]string, error) {
	if disp == nil {
		return nil, nil
	}

	count, err := getPropertyInt32(disp, "Count")
	if err != nil {
		return nil, fmt.Errorf("Count: %w", err)
	}

	stringCollection := make([]string, count)

	for i := 0; i < int(count); i++ {
		str, err := getPropertyString(disp, "Item", i)
		if err != nil {
			return nil, fmt.Errorf("Item[%d/%d]: %w", i, count, err)
		}

		stringCollection[i] = str
	}
	return stringCollection, nil
}
