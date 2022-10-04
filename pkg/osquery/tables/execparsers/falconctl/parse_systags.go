package falconctl

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// parseSystags parses the systags. As we understand it, this is a simple array of strings. One per line. Maybe comma delimited.
func parseSystags(reader io.Reader) (any, error) {
	found := make([]string, 0)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		for _, chunk := range strings.Split(line, ",") {

			// trim quotes and spaces
			chunk = strings.TrimSpace(strings.Trim(chunk, `"`))

			// If a chunk has a space in the middle, it's malformed and we should error out
			if strings.Contains(chunk, " ") {
				return nil, fmt.Errorf("malformed chunk: %s in line %s", chunk, line)
			}

			found = append(found, chunk)
		}

	}

	return found, nil
}
