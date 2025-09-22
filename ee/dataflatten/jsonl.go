package dataflatten

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func JsonlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("unable to read JSON file: %w", err)
	}

	return flattenJsonl(rawdata, opts...)
}

func Jsonl(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	return flattenJsonl(rawdata, opts...)
}

func flattenJsonl(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawdata))
	var objects []interface{}

	for {
		var object interface{}
		err := decoder.Decode(&object)

		switch {
		case err == nil:
			objects = append(objects, object)
		case err == io.EOF:
			return Flatten(objects, opts...)
		default:
			return nil, fmt.Errorf("unmarshalling jsonl: %w", err)
		}
	}
}
