//go:build darwin
// +build darwin

package plist

import (
	"io"
)

// parser implements the parser interface for plist data
type parser struct {
}

// Parser is the singleton instance of the parser
var Parser = New()

// New creates a new parser instance
func New() *parser {
	return &parser{}
}

// Parse implements the parser interface
// It takes a reader containing plist data and returns a structured representation
func (p *parser) Parse(reader io.Reader) (any, error) {
	return p.parsePlist(reader)
}
