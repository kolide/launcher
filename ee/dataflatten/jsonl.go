package dataflatten

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func JsonlFile(file string, opts ...FlattenOpts) ([]Row, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("unable to open JSON file: %w", err)
	}
	defer f.Close()

	return flattenJsonl(f, opts...)
}

func Jsonl(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	dataReader := bytes.NewReader(rawdata)
	return flattenJsonl(dataReader, opts...)
}

func flattenJsonl(rawdataReader io.Reader, opts ...FlattenOpts) ([]Row, error) {
	decoder := json.NewDecoder(rawdataReader)
	var objects []any

	prg, err := NewCELPrefilter(hardcodedCELPrefilter)
	if err != nil {
		return nil, fmt.Errorf("initializing prefilter: %w", err)
	}

	for {
		var object any
		err := decoder.Decode(&object)

		switch err {
		case nil:
			filteredObj, err := RunCELPrefilter(prg, object)
			if err != nil {
				return nil, fmt.Errorf("prefiltering object prior to flattening: %w", err)
			}
			if filteredObj != nil {
				objects = append(objects, filteredObj)
			}
		case io.EOF:
			return Flatten(objects, opts...)
		default:
			return nil, fmt.Errorf("unmarshalling jsonl: %w", err)
		}
	}
}
