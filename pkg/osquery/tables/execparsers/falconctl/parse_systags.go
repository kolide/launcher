package falconctl

import (
	"bufio"
	"io"
	"strings"
)

// parseSystags parses the systags. As we understand it, this is a simple array of strings. One per line. Maaaybe comma delimited.
func parseSystags(reader io.Reader) (any, error) {
	found := make([]string, 10)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		for _, chunk := range strings.Split(line, ",") {

			// remove quotes and spaces
			chunk = strings.TrimSpace(strings.Trim(chunk, `"`))

			found = append(found, chunk)
		}

	}

	return found, nil
}
