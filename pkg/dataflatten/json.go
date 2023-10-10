package dataflatten

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func JsonFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	// Attempt to decode utf16 data.
	valid_json := json.Valid(rawdata)
	if !valid_json {
		rawdata, _, err = transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), rawdata)
		if err != nil {
			return nil, err
		}
	}

	return Json(rawdata, opts...)
}

func Json(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	var data interface{}

	if err := json.Unmarshal(rawdata, &data); err != nil {
		return nil, fmt.Errorf("unmarshalling json: %w", err)
	}

	return Flatten(data, opts...)
}
