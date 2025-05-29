//go:build darwin || linux
// +build darwin linux

package json

import (
	"io"
)

type parser struct {
}

var Parser = New()

func New() *parser {
	return &parser{}
}

func (p *parser) Parse(reader io.Reader) (any, error) {
	return p.parseJson(reader)
}
