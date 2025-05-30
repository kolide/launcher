package json

import (
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// parseJson implements the parsing logic for JSON data.
// It follows the same approach as ee/dataflatten/json.go
func (p *parser) parseJson(reader io.Reader) (any, error) {
	// Read all data from the reader
	rawdata, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading from reader: %w", err)
	}

	var data any

	// Check if the raw data is valid JSON
	if json.Valid(rawdata) {
		if err := json.Unmarshal(rawdata, &data); err != nil {
			return nil, fmt.Errorf("unmarshalling json: %w", err)
		}
		return data, nil
	}

	// We don't have valid json data, so try to convert possible utf16 data to utf8
	rawdata, _, err = transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), rawdata)
	if err != nil {
		return nil, fmt.Errorf("transforming invalid json from utf16 to utf8: %w", err)
	}

	// Try to unmarshal the transformed data
	if err := json.Unmarshal(rawdata, &data); err != nil {
		return nil, fmt.Errorf("unmarshalling json after transforming from utf16 to utf8: %w", err)
	}

	return data, nil
}
