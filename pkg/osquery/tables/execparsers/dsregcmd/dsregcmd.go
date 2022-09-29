package dsregcmd

import "io"

type Parser struct{}

func New() Parser {
	return Parser{}
}

func (p Parser) Parse(reader io.Reader) (any, error) {
	return parse(reader)
}
