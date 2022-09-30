package dataflattentable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kolide/launcher/pkg/dataflatten"
)

type parserInt interface {
	Parse(io.Reader) (any, error)
}

// parserFlattener is a simple wrapper over a parser, to convert it to a flattener.
type parserFlattener struct {
	parser parserInt
}

func flattenerFromParser(parser parserInt) parserFlattener {
	return parserFlattener{parser: parser}
}

func (p parserFlattener) FlattenBytes(raw []byte, flattenOpts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	data, err := p.parser.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("error parsing data: %w", err)
	}

	// Data comes in as map[string]map[string]interface{}, but Flatten expects a map[string]interface{}. I'm not sure
	// how to resolve this, so for now we round trip through Json.
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshalling json: %w", err)
	}

	return dataflatten.Json(jsonBytes, flattenOpts...)
}
