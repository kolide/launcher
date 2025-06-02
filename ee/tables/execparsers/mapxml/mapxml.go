//go:build darwin || linux
// +build darwin linux

package mapxml

import (
	"bufio"
	"io"
)

// parser implements the parser interface for XML data from nix-env
type parser struct {
	scanner      *bufio.Scanner
	lastReadLine string
}

// Parser is the singleton instance of the parser
var Parser = New()

// New creates a new parser instance
func New() *parser {
	return &parser{}
}

// Parse implements the parser interface
// It takes a reader containing XML data from nix-env and returns a structured representation
func (p *parser) Parse(reader io.Reader) (any, error) {
	return p.parseXml(reader)
}
