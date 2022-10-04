package simple_array

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type parser struct {
}

func (p parser) Parse(reader io.Reader) (any, error) {
	return parse(reader)
}

func New() parser {
	return parser{}
}

// Parser is a parser for dsregcmd output
var Parser = New()

// parser parses line by line, additionally splitting on commas. As this is an array of simple strings, anything with a space is an error
func parse(reader io.Reader) (any, error) {
	found := make([]string, 0)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		for _, chunk := range strings.Split(line, ",") {

			// trim quotes and spaces
			chunk = strings.TrimSpace(chunk)
			chunk = strings.Trim(chunk, `"`)

			// If a chunk has a space in the middle, it's malformed and we should error out
			if strings.Contains(chunk, " ") {
				return nil, fmt.Errorf("malformed chunk: %s in line %s", chunk, line)
			}

			found = append(found, chunk)
		}
	}

	return found, nil
}
