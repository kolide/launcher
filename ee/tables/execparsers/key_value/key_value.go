//go:build darwin || linux
// +build darwin linux

package key_value

import (
	"io"
)

// parser implements the parser interface for key-value data
type parser struct {
	kvDelimiter string
}

// Parser is the singleton instance of the parser
var Parser = New()

// New creates a new parser instance with default delimiter
func New() *parser {
	return &parser{
		kvDelimiter: "=", // default delimiter
	}
}

// NewWithDelimiter creates a new parser instance with a custom delimiter
func NewWithDelimiter(delimiter string) *parser {
	return &parser{
		kvDelimiter: delimiter,
	}
}

// SetDelimiter sets the key-value delimiter for the parser
func (p *parser) SetDelimiter(delimiter string) {
	p.kvDelimiter = delimiter
}

// Parse implements the parser interface
// It takes a reader containing key-value data and returns a structured representation
func (p *parser) Parse(reader io.Reader) (any, error) {
	return p.parseKeyValue(reader)
}
