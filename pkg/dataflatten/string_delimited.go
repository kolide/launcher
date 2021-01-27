package dataflatten

import (
	"bufio"
	"bytes"
	"strings"
)

func StringDelimited(rawdata []byte, delimiter string, opts ...FlattenOpts) ([]Row, error) {
	return flattenStringDelimited(rawdata, delimiter, opts...)
}

func flattenStringDelimited(in []byte, delimiter string, opts ...FlattenOpts) ([]Row, error) {
	v := map[string]interface{}{}
	scanner := bufio.NewScanner(bytes.NewReader(in))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, delimiter, 2)
		if len(parts) < 2 {
			continue
		}
		k := parts[0]
		// TODO: the parent key is never set...
		v[k] = strings.TrimSpace(parts[1])
	}

	return Flatten(v, opts...)
}
