package dataflatten

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

func JsonlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("unable to open JSONL file: %w", err)
	}
	defer f.Close()

	return flattenJsonl(f, opts...)
}

func Jsonl(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	dataReader := bytes.NewReader(rawdata)
	return flattenJsonl(dataReader, opts...)
}

func flattenJsonl(r io.Reader, opts ...FlattenOpts) ([]Row, error) {
	dec := json.NewDecoder(r)
	// Use FlattenEach to handle flattening (and prefiltering) as we decode each object,
	// so that we can immediately discard objects that are prefiltered out
	return FlattenEach(func(yield func(any, error) bool) {
		for {
			var obj any
			if err := dec.Decode(&obj); err != nil {
				if !errors.Is(err, io.EOF) {
					yield(nil, fmt.Errorf("unmarshalling jsonl: %w", err))
				}

				return
			}
			if !yield(obj, nil) {
				return
			}
		}
	}, opts...)
}
