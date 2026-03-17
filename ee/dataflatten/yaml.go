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

	docs := make([]any, 0)
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

		docs = append(docs, data)
	}

	// If we only have one document, no need to prepend an index of 0 to the path.
	if len(docs) == 1 {
		return Flatten(docs[0], opts...)
	}

	return Flatten(docs, opts...)
}
