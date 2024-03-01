package dataflatten

import (
	"bufio"
	"bytes"
	"strings"
)

type dataFunc func(data []byte, opts ...FlattenOpts) ([]Row, error)

type recordSplittingStrategy int

const (
	None recordSplittingStrategy = iota
	DuplicateKeys
)

// StringDelimitedLineFunc returns a function that conforms to the interface expected by
// dataflattentable.Table's execDataFunc property. Properties are grouped into a single
// record per line of data by splitting the line into fields based on a separator.
//
// Headers are expected to either be provided, or set from the first line of data using the separator.
// If no separator is provided, it's assumed that fields are separated by whitespace.
// The first N lines of data can be skipped in case garbage is sent with the data.
func StringDelimitedLineFunc(sep string, headers []string, skipN int) dataFunc {
	return func(data []byte, opts ...FlattenOpts) ([]Row, error) {
		results := []interface{}{}
		scanner := bufio.NewScanner(bytes.NewReader(data))

		// Skip first N lines due to provided headers or otherwise. This would likely be 1 or 0.
		for skipN > 0 {
			if scanner.Scan() {
				skipN--
			} else {
				// Early exit if the scanner skipped past all data.
				return Flatten(results, opts...)
			}
		}

		header_count := len(headers)

		for scanner.Scan() {
			line := scanner.Text()

			// headers weren't provided, so retrieve them from the first available line.
			if header_count == 0 {
				headers = lineSplit(sep, line, header_count)
				header_count = len(headers)
				continue
			}

			row := map[string]interface{}{}
			fields := lineSplit(sep, line, header_count)
			// It's possible we don't have the same number of fields to headers, so use
			// min here to avoid a possible array out-of-bounds exception.
			min := min(header_count, len(fields))

			// For each header, add the corresponding line field to the result row.
			for c := 0; c < min; c++ {
				row[strings.TrimSpace(headers[c])] = strings.TrimSpace(fields[c])
			}

			results = append(results, row)
		}

		return Flatten(results, opts...)
	}
}

// Switch to the appropriate function to return the current line's fields.
// Separator often might be a comma or similar single character delimiter.
func lineSplit(sep string, line string, count int) []string {
	switch sep {
	case "":
		// Separator wasn't provided, assume whitespace delimited fields.
		return strings.Fields(line)
	default:
		// If we have a count of the headers, split the current line to N fields.
		// Otherwise assume headers weren't provided and split the initial line to set them.
		if count > 0 {
			return strings.SplitN(sep, line, count)
		} else {
			return strings.Split(line, sep)
		}
	}
}

func StringDelimitedFunc(kVDelimiter string, splittingStrategy recordSplittingStrategy) dataFunc {
	switch splittingStrategy {
	case None:
		return singleRecordFunc(kVDelimiter)
	case DuplicateKeys:
		return duplicateKeyFunc(kVDelimiter)
	default:
		panic("Unknown record splitting strategy")
	}
}

// duplicateKeyFunc returns a function that conforms to the interface expected
// by dataflattentable.Table's execDataFunc property. properties are grouped
// into a single record based on 'duplicate key' strategy: If a key/value pair
// is encountered, and the record being built already has a value for that key,
// then that record is considered 'complete'. The record is stored in the
// collection, and a new record is started. This strategy is only suitable if
// properties for a single record are grouped together, and there is at least
// one field that appears for every record before any sparse data.
func duplicateKeyFunc(kVDelimiter string) dataFunc {
	return func(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
		results := []interface{}{}
		scanner := bufio.NewScanner(bytes.NewReader(rawdata))
		row := map[string]interface{}{}
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, kVDelimiter, 2)
			if len(parts) < 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if _, ok := row[key]; ok { // this key already exists, so we want to start a new record.
				results = append(results, row) // store the 'finished' record in the collection
				row = map[string]interface{}{} // reset the record
			}
			row[key] = value
		}
		results = append(results, row) // store the final record

		return Flatten(results, opts...)
	}
}

// singleRecordFunc returns an execData function that assumes 'rawdata'
// only holds key-value pairs for a single record. Additionally, each k/v pair
// must be on its own line. Useful for output that can be easily separated into
// separate records before 'flattening'
func singleRecordFunc(kVDelimiter string) dataFunc {
	return func(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
		results := []interface{}{}
		scanner := bufio.NewScanner(bytes.NewReader(rawdata))
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, kVDelimiter, 2)
			if len(parts) < 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			results = append(results, map[string]interface{}{key: value})
		}

		return Flatten(results, opts...)
	}
}
