package dsregcmd

import "io"

type Parser struct{}

func (p Parser) Parse(reader io.Reader) (any, error) {
	return Parse(reader)
}
