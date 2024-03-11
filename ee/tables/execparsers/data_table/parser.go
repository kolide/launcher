package data_table

import (
	"bufio"
	"io"
	"strings"
)

// data_table is a general parser for an input of data which conforms to columns and rows styled output.
// Parser options
// skipLines - The number of initial lines of data to skip. By default no lines are skipped. This can be useful if consistent undesired output/garbage is printed before the data to parse.
// headers - The set of headers. If left blank, the parser assumes the headers are in the first line of data and splits the line to set them.
// delimiter - The splitting string. If left blank, the parser assumes the delimiter is whitespace and uses `strings.Fields()` split method.
type parser struct {skipLines uint, headers []string, delimiter string}

func Parser(skipLines uint, headers []string, delimiter string) parser {
	return parser{skipLines: skipLines, headers: headers, delimiter: delimiter}
}

func (p parser) Parse(reader io.Reader) (any, error) {
	return parseLines(reader)
}

// parseLines scans a reader line by line and splits it into fields based on a delimiter.
// The line fields are paired with a header, which is defined by an input array, or the first line of data.
// If no delimiter is provided, it's assumed that fields are separated by whitespace.
// The first N lines of data can be skipped in case garbage is sent before the data.
func (p parser) parseLines(reader io.Reader) []map[string]string {
	results := make([]map[string]string, 0)
	scanner := bufio.NewScanner(reader)

	// Skip first N lines due to provided headers or otherwise.
	// This would likely only ever be 1 or 0, but we may want more.
	for p.skipLines > 0 {
		p.skipLines--
		// Early exit if the scanner skipped past all data.
		if !scanner.Scan() {
			return results // <-- do we want to error here?
		}
	}

	headerCount := len(p.headers)

	for scanner.Scan() {
		line := scanner.Text()

		// headers weren't provided, so retrieve them from the first available line.
		if headerCount == 0 {
			p.headers = lineSplit(line, headerCount)
			headerCount = len(p.headers)
			continue
		}

		row := map[string]string
		fields := lineSplit(line, headerCount)
		// It's possible we don't have the same number of fields to headers, so use
		// min here to avoid a possible array out-of-bounds exception.
		min := min(headerCount, len(fields))

		// For each header, add the corresponding line field to the result row.
		for c := 0; c < min; c++ {
			row[strings.TrimSpace(p.headers[c])] = strings.TrimSpace(fields[c])
		}

		results = append(results, row)
	}

	return results
}

// Switch to the appropriate function to return the current line's fields.
// Delimiter often might be a comma or similar single character.
func (p parser) lineSplit(line string, headerCount int) []string {
	switch p.delimiter {
	case "":
		// Delimiter wasn't provided, assume whitespace separated fields.
		return strings.Fields(line)
	default:
		// If we have a count of the headers, split the current line to N fields.
		// Otherwise assume headers weren't provided and split the initial line to set them.
		if headerCount > 0 {
			return strings.SplitN(p.delimiter, line, headerCount)
		} else {
			return strings.Split(line, p.delimiter)
		}
	}
}
