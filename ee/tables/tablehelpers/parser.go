package tablehelpers

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
)

type Matcher struct {
	Match   func(string) bool
	KeyFunc func(string) (string, error)
	ValFunc func(string) (string, error)
}

type OutputParser struct {
	matchers []Matcher
	slogger  *slog.Logger
}

func NewParser(slogger *slog.Logger, matchers []Matcher) *OutputParser {
	p := &OutputParser{
		matchers: matchers,
		slogger:  slogger,
	}
	return p
}

// Parse looks at a chunk of input. It is assumed that the input contains
// information to fill in a single result. Do not provide input that contains
// data for multiple results.
func (p *OutputParser) Parse(input *bytes.Buffer) map[string]string {
	row := make(map[string]string)
	scanner := bufio.NewScanner(input)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		for _, m := range p.matchers {
			if m.Match(line) {
				key, err := m.KeyFunc(line)
				if err != nil {
					p.slogger.Log(context.TODO(), slog.LevelDebug,
						"key match failed",
						"line", line,
						"err", err,
					)
					continue
				}

				val, err := m.ValFunc(line)
				if err != nil {
					p.slogger.Log(context.TODO(), slog.LevelDebug,
						"value match failed",
						"line", line,
						"err", err,
					)
					continue
				}

				row[key] = val
				continue
			}
		}
	}
	return row
}

// Parse looks at command output, line by line. It uses the defined Matchers to set any appropriate values
func (p *OutputParser) ParseMultiple(input *bytes.Buffer) []map[string]string {
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
					p.slogger.Log(context.TODO(), slog.LevelDebug,
						"key match failed",
						"line", line,
						"err", err,
					)
					continue
				}

				val, err := m.ValFunc(line)
				if err != nil {
					p.slogger.Log(context.TODO(), slog.LevelDebug,
						"value match failed",
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
			p.slogger.Log(context.TODO(), slog.LevelDebug,
				"no matched keys",
				"line", line,
			)
			continue
		}
		results = append(results, row)

	}
	if err := scanner.Err(); err != nil {
		p.slogger.Log(context.TODO(), slog.LevelDebug,
			"scanner error",
			"err", err,
		)
	}
	return results
}
