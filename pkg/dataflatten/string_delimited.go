package dataflatten

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

type dataFunc func(data []byte, opts ...FlattenOpts) ([]Row, error)

// StringDelimitedUnseparatedFunc returns a function that conforms to the
// interface expected by dataflattentable.Table's execDataFunc property.
// properties are grouped into a single record based on 'duplicate key'
// strategy: If a key/value pair is encountered, and the record being built
// already has a value for that key, then that record is considered 'complete'.
// The record is stored in the collection, and a new record is started. This
// strategy is only suitable if the data output does not exclude k/v pairs with
// blank/missing values, and assumes that the properties for a single record are
// grouped together.
func StringDelimitedUnseparatedFunc(delimiter string) dataFunc {
	return func(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
		v := map[string]interface{}{}
		scanner := bufio.NewScanner(bytes.NewReader(rawdata))
		row := map[string]interface{}{}
		i := 0
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, delimiter, 2)
			if len(parts) < 2 {
				continue
			}
			key := parts[0]
			value := strings.TrimSpace(parts[1])
			if _, ok := row[key]; ok { // this key already exists, so we want to start a new record.
				v[strconv.Itoa(i)] = row // store the 'finished' record in the collection
				i++
				row = map[string]interface{}{} // reset the record
			}
			row[key] = value
		}
		v[strconv.Itoa(i)] = row // store the final record

		return Flatten(v, opts...)
	}
}

// StringDelimited assumes that rawdata only holds key-value pairs for a single
// record. Additionally, each k/v pair must be on its own line. Useful for
// output that can be easily separated into separate records before 'flattening'
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
		v[k] = strings.TrimSpace(parts[1])
	}

	return Flatten(v, opts...)
}
