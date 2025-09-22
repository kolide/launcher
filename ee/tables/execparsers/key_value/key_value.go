package key_value

import (
	"io"
)

type parser struct {
	kvDelimiter string
}

func New() *parser {
	return &parser{
		kvDelimiter: "=", // default delimiter
	}
}

func NewWithDelimiter(delimiter string) *parser {
	return &parser{
		kvDelimiter: delimiter,
	}
}

func (p *parser) SetDelimiter(delimiter string) {
	p.kvDelimiter = delimiter
}

func (p *parser) Parse(reader io.Reader) (any, error) {
	return p.parseKeyValue(reader)
}
