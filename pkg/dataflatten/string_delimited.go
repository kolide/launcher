package dataflatten

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

func StringDelimited(rawdata []byte, delimiter string, opts ...FlattenOpts) ([]Row, error) {
	return flattenStringDelimited(rawdata, delimiter, opts...)
}

type dataFunc func(data []byte, opts ...FlattenOpts) ([]Row, error)

// StringDelimitedUnseparatedFunc returns a function that conforms to the
// function expected by dataflattentable.Table's execDataFunc property
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
				// TODO: this is potentially problematic if the output doesn't
				// include a separator for blank values. we need to record keys even
				// if there is no value for the 'split records by duplicate keys'
				// strategy to work
				// level.Debug(logger).Log("msg", "not enough parts to get a k/v pair", "line", line)
				continue
			}
			key := parts[0]
			value := strings.TrimSpace(parts[1])
			if _, ok := row[key]; ok { // this key already exists, so we want to start a new record.
				v[strconv.Itoa(i)] = row // record the results in the collection
				i++
				row = map[string]interface{}{} // reset the record
			}
			row[key] = value
		}
		v[strconv.Itoa(i)] = row // store the final result

		return Flatten(v, opts...)
	}
}

// StringDelimitedUnseparated will decide when to create a new record based on
// when a duplicate key is found.
func StringDelimitedUnseparated(rawdata []byte, delimiter string, opts ...FlattenOpts) ([]Row, error) {
	v := map[string]interface{}{}
	scanner := bufio.NewScanner(bytes.NewReader(rawdata))
	row := map[string]interface{}{}
	i := 0
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, delimiter, 2)
		if len(parts) < 2 {
			// TODO: this is potentially problematic if the output doesn't
			// include a separator for blank values. we need to record keys even
			// if there is no value for the 'split records by duplicate keys'
			// strategy to work
			// level.Debug(logger).Log("msg", "not enough parts to get a k/v pair", "line", line)
			continue
		}
		key := parts[0]
		value := strings.TrimSpace(parts[1])
		if _, ok := row[key]; ok { // this key already exists, so we want to start a new record.
			v[strconv.Itoa(i)] = row // record the results in the collection
			i++
			row = map[string]interface{}{} // reset the record
		}
		row[key] = value
	}
	v[strconv.Itoa(i)] = row // store the final result

	return Flatten(v, opts...)
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
