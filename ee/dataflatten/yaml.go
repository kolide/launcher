package dataflatten

import (
	"bytes"
	"fmt"
	"io"

	"go.yaml.in/yaml/v4"
)

func Yaml(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	// We use the loader to accommodate multiple documents per single yaml file
	reader := bytes.NewReader(rawdata)
	loader, err := yaml.NewLoader(reader)
	if err != nil {
		return nil, fmt.Errorf("loading yaml: %w", err)
	}

	rows := make([]Row, 0)
	for {
		var data any
		err := loader.Load(&data)
		if err == io.EOF {
			// No more documents
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing document: %w", err)
		}

		newRows, err := Flatten(data, opts...)
		if err != nil {
			return nil, fmt.Errorf("flattening document: %w", err)
		}
		rows = append(rows, newRows...)
	}

	return rows, nil
}
