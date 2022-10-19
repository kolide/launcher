//go:build darwin
// +build darwin

package remotectl

import "io"

type parser struct{}

var Parser = New()

func New() parser {
	return parser{}
}

func (p parser) Parse(reader io.Reader) (any, error) {
	return parseDumpstate(reader)
}
