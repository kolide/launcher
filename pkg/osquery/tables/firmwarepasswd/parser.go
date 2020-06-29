package firmwarepasswd

import (
	"bufio"
	"bytes"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
)

type Matcher struct {
	Match   func(string) bool
	KeyFunc func(string) (string, error)
	ValFunc func(string) (string, error)
}

type OutputParser struct {
	matchers []Matcher
	logger   log.Logger
}

func NewParser(logger log.Logger, matchers []Matcher) *OutputParser {
	p := &OutputParser{
		matchers: matchers,
		logger:   logger,
	}
	return p
}

// Parse looks at command output, line by line. It uses the defined Matchers to set any appropriate values
func (p *OutputParser) Parse(input *bytes.Buffer) []map[string]string {
	var results []map[string]string

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		row := make(map[string]string)

		// check each possible key match
		for _, m := range p.matchers {
			if m.Match(line) {
				key, err := m.KeyFunc(line)
				if err != nil {
					level.Debug(p.logger).Log(
						"msg", "key match failed",
						"line", line,
						"err", err,
					)
					continue
				}

				val, err := m.ValFunc(line)
				if err != nil {
					level.Debug(p.logger).Log(
						"msg", "value match failed",
						"line", line,
						"err", err,
					)
					continue
				}

				row[key] = val
				continue
			}
		}

		if len(row) == 0 {
			level.Debug(p.logger).Log("msg", "No matched keys", "line", line)
			continue
		}
		results = append(results, row)

	}
	if err := scanner.Err(); err != nil {
		level.Debug(p.logger).Log("msg", "scanner error", "err", err)
	}
	return results
}

func discernValBool(in string) (bool, error) {
	switch strings.TrimSpace(strings.ToLower(in)) {
	case "true", "t", "1", "y", "yes":
		return true, nil
	case "false", "f", "0", "n", "no":
		return false, nil
	}

	return false, errors.Errorf("Can't discern boolean from string <%s>", in)
}
