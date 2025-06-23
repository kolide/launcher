//go:build darwin || linux
// +build darwin linux

package mapxml

import (
	"io"

	"github.com/clbanning/mxj"
)

// parseXml implements the parser interface for XML data
func (p *parser) parseXml(reader io.Reader) (any, error) {
	// Use the mxj library to parse XML directly from the reader
	mv, err := mxj.NewMapXmlReader(reader)
	if err != nil {
		return nil, err
	}

	// Return the parsed data as a map
	return mv.Old(), nil
}
