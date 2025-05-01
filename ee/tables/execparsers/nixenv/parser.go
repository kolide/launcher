package nixenv

import (
	"io"

	"github.com/clbanning/mxj"
)

// parseNixXml implements the parser interface for nix-env XML data
func (p *parser) parseNixXml(reader io.Reader) (any, error) {
	// Read all data from the reader
	rawdata, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// Use the existing mxj library to parse XML
	mv, err := mxj.NewMapXml(rawdata)
	if err != nil {
		return nil, err
	}

	// Return the parsed data as a map
	return mv.Old(), nil
}
