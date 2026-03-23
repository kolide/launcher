package dataflatten

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func JsoncFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}

	transformedRawdata, err := jsoncToJson(rawdata)
	if err != nil {
		return nil, fmt.Errorf("transforming JSONC to JSON: %w", err)
	}

	if json.Valid(transformedRawdata) {
		// We call Jsonl rather than Jsonc because we know it's already valid transformed JSON
		return Jsonl(transformedRawdata)
	}

	// We still don't have valid json data -- next try to convert possible utf16 data to utf8.
	transformedRawdata, _, err = transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), transformedRawdata)
	if err != nil {
		return nil, fmt.Errorf("attempting to transform invalid json from utf16 to utf8: %w", err)
	}

	return Jsonl(transformedRawdata, opts...)
}

func Jsonc(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := jsoncToJson(rawdata)
	if err != nil {
		return nil, fmt.Errorf("converting jsonc to json: %w", err)
	}

	return Jsonl(rawdata, opts...)
}

// jsoncToJson takes the JSONC contained in `rawData` and strips out comments and trailing commas,
// so that it can be parsed as JSON.
// Single-line comments start with // and extend to the end of the line.
// Multi-line comments start with /* and end with */. They can span multiple lines.
// See specification: https://jsonc.org/
func jsoncToJson(rawData []byte) ([]byte, error) {
	// We wrap the byte reader in the bufio reader in order to be able to call Peek
	rawDataReader := bufio.NewReader(bytes.NewReader(rawData))
	out := make([]byte, len(rawData))

	currentOutputIndex := 0
	insideString := false
	for {
		currentByte, err := rawDataReader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out[:currentOutputIndex], nil
			}
			return nil, fmt.Errorf("reading next byte: %w", err)
		}

		// First, check if we're in a string -- we want to ignore comment chars when inside strings.
		// If there was a char prior to this one, we need to make sure it wasn't a backslash escaping the quotation mark.
		if currentByte == '"' && (currentOutputIndex == 0 || out[currentOutputIndex-1] != '\\') {
			if insideString {
				insideString = false
			} else {
				insideString = true
			}
		}

		// Handle start of both types of comments
		if !insideString && currentByte == '/' {
			nextByte, err := rawDataReader.Peek(1)
			if err != nil {
				return nil, fmt.Errorf("peeking ahead after `/`: %w", err)
			}
			if len(nextByte) != 1 {
				return nil, fmt.Errorf("peeking ahead 1 byte returned unexpected number of bytes %d", len(nextByte))
			}
			switch nextByte[0] {
			case '/':
				// Single-line comment -- read and discard until end of line
				for {
					currentByte, err = rawDataReader.ReadByte()
					if err != nil {
						// Handle single-line comment at end of file without LF after
						if errors.Is(err, io.EOF) {
							return out[:currentOutputIndex], nil
						}
						return nil, fmt.Errorf("reading and discarding single-line comment: %w", err)
					}
					if currentByte == '\n' || currentByte == '\r' {
						break
					}
				}
			case '*':
				// Opening of multi-line comment -- read and discard until we see `*/`
				for {
					currentByte, err = rawDataReader.ReadByte()
					if err != nil {
						return nil, fmt.Errorf("reading and discarding multi-line comment: %w", err)
					}
					if currentByte != '*' {
						continue
					}

					// Check to see if / comes next by reading the next byte
					currentByte, err = rawDataReader.ReadByte()
					if err != nil {
						return nil, fmt.Errorf("reading and discarding multi-line comment after *: %w", err)
					}
					if currentByte == '/' {
						// End of comment -- read our next byte
						currentByte, err = rawDataReader.ReadByte()
						if err != nil {
							// Handle multi-line comment at end of file without LF after
							if errors.Is(err, io.EOF) {
								return out[:currentOutputIndex], nil
							}
							return nil, fmt.Errorf("reading next byte after close of multi-line comment: %w", err)
						}
						break
					}
				}
			}
		}

		// Check for trailing commas -- a comma that immediately precedes a ] or }. If we looked forward
		// from every comma to check if the next non-whitespace char is ] or }, we'd also have to handle
		// parsing comments here (because a sequence of trailing comma + comment + ] or } is still a trailing comma).
		// So instead, we look backward in `out`, since `out` already has the comments stripped out of it.
		if !insideString && (currentByte == ']' || currentByte == '}') {
			// Check if our last non-whitespace byte was `,`
			for i := currentOutputIndex - 1; i > -1; i-- {
				if out[i] == ' ' || out[i] == '\r' || out[i] == '\n' || out[i] == '\t' {
					continue
				}
				if out[i] != ',' {
					// No trailing comma detected, nothing to do
					break
				}

				// Substitute a space for the trailing comma
				out[i] = ' '
				break
			}
		}

		out[currentOutputIndex] = currentByte
		currentOutputIndex += 1
	}
}
