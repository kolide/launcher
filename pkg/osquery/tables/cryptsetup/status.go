package cryptsetup

import (
	"bufio"
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
)

// parseStatus parses the output from `cryptsetup status`. This is a
// pretty simple key, value format, but does have a free form first
// line. It's not clear if this is going to be stable, or change
// across versions.
func parseStatus(rawdata []byte) (map[string]string, error) {
	var data map[string]string

	scanner := bufio.NewScanner(bytes.NewReader(rawdata))
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			var err error
			data, err = parseFirstLine(line)
			if err != nil {
				return nil, err
			}

			firstLine = false
			continue
		}

		kv := strings.SplitN(line, ": ", 2)
		data[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}

	return data, nil
}

// Device sdc3 not found
var firstLineRegexp = regexp.MustCompile(`^(?:Device (.*) (not found))|(?:(.*?) is ([a-z]+)(?:\.| and is (in use)))`)

func parseFirstLine(line string) (map[string]string, error) {
	if line == "" {
		return nil, nil
	}

	m := firstLineRegexp.FindAllStringSubmatch(line, -1)
	if len(m) != 1 {
		return nil, errors.Errorf("Failed to match first line: %s", line)
	}
	if len(m[0]) != 6 {
		spew.Dump(m)
		return nil, errors.Errorf("Got %d matches. Expected 6. Failed to match first line: %s", len(m[0]), line)
	}

	data := make(map[string]string, 3)

	// check for $1 and $2 for the error condition
	if m[0][1] != "" && m[0][2] != "" {
		data["short_name"] = m[0][1]
		data["status"] = strings.ReplaceAll(m[0][2], " ", "_")
		data["mounted"] = strconv.FormatBool(false)
		return data, nil
	}

	if m[0][3] != "" && m[0][4] != "" {
		data["display_name"] = m[0][3]
		data["status"] = strings.ReplaceAll(m[0][4], " ", "_")
		data["mounted"] = strconv.FormatBool(m[0][5] != "")
		return data, nil
	}

	return nil, errors.Errorf("Unknown first line: %s", line)
}
