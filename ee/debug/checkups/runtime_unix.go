//go:build linux || darwin

package checkups

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func (c *runtimeCheckup) findDesktopProcessesWithLsof() ([]desktopProcessInfo, error) {
	// Use lsof to find processes with desktop socket files
	cmd, err := allowedcmd.Lsof(context.Background(), "-U", "-a", "-c", "launcher")
	if err != nil {
		return nil, fmt.Errorf("creating lsof command: %w", err)
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running lsof: %w", err)
	}

	var processes []desktopProcessInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		// Look for desktop.sock in the socket path
		socketPath := fields[7] // Last field should be the socket path
		if !strings.Contains(socketPath, "desktop.sock") {
			continue
		}

		// Extract PID
		pidStr := fields[1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Get auth token from process environment
		authToken, err := c.getAuthTokenFromProcess(pid)
		if err != nil {
			continue // Skip processes where we can't get the token
		}

		processes = append(processes, desktopProcessInfo{
			socketPath: socketPath,
			authToken:  authToken,
		})
	}

	return processes, nil
}

func (c *runtimeCheckup) getAuthTokenFromProcess(pid int) (string, error) {
	// Read process environment
	cmd, err := allowedcmd.Ps(context.Background(), "eww", strconv.Itoa(pid))
	if err != nil {
		return "", fmt.Errorf("creating ps command: %w", err)
	}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading process environment: %w", err)
	}

	// Parse environment variables from ps output
	envVars := strings.Fields(string(output))
	for _, envVar := range envVars {
		if strings.HasPrefix(envVar, "USER_SERVER_AUTH_TOKEN=") {
			return strings.TrimPrefix(envVar, "USER_SERVER_AUTH_TOKEN="), nil
		}
	}

	return "", errors.New("auth token not found in process environment")
}
