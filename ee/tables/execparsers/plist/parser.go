//go:build darwin
// +build darwin

package plist

import (
	"errors"
	"io"

	"howett.net/plist"
)

// parsePlist implements the parser interface for plist data
func (p *parser) parsePlist(reader io.Reader) (any, error) {
	// Read all data from the reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// Check for empty input
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	// Create a variable to hold the parsed data
	var result interface{}

	// Unmarshal the plist data
	if _, err := plist.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	// Return the parsed data
	return result, nil
}
