//go:build darwin || !cgo
// +build darwin !cgo

package table

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/kolide/launcher/pkg/allowedpaths"
)

func mdfind(args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd, err := allowedpaths.Mdfind(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("creating mdfind command: %w", err)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var lines []string
	lr := bufio.NewReader(bytes.NewReader(out))
	for {
		line, _, err := lr.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, string(line))
	}
	return lines, nil
}
