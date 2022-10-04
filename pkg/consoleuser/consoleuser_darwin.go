//go:build darwin
// +build darwin

package consoleuser

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func CurrentUids(context context.Context) ([]string, error) {
	cmd := exec.CommandContext(context, "scutil")
	cmd.Stdin = strings.NewReader("show State:/Users/ConsoleUser")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("executing scutl cmd: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		const uidKey = "UID : "

		if !strings.Contains(line, uidKey) {
			continue
		}

		parts := strings.Split(line, uidKey)

		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected output from scutil: %s", line)
		}

		return []string{parts[1]}, nil
	}

	// there is no console user
	return []string{}, nil
}
