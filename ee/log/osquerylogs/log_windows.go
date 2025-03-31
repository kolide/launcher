//go:build windows
// +build windows

package osquerylogs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

func (l *OsqueryLogAdapter) runAndLogPs(_ string) {
	return
}

func (l *OsqueryLogAdapter) runAndLogLsofByPID(_ string) {
	return
}

func (l *OsqueryLogAdapter) runAndLogLsofOnPidfile() {
	return
}

func getProcessesHoldingFile(ctx context.Context, pathToFile string) ([]*process.Process, error) {
	allProcesses, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting process list: %w", err)
	}
	if len(allProcesses) == 0 {
		return nil, errors.New("could not get any processes")
	}

	processes := make([]*process.Process, 0)
	for _, p := range allProcesses {
		openFiles, err := p.OpenFilesWithContext(ctx)
		if err != nil {
			continue
		}

		// Check the process's open files to see if this process is the one using the lockfile
		for _, f := range openFiles {
			// We check for strings.Contains rather than equals because the open file's path contains
			// a `\\?\` prefix.
			if !strings.Contains(f.Path, pathToFile) {
				continue
			}

			processes = append(processes, p)
			break
		}
	}

	if len(processes) == 0 {
		return nil, fmt.Errorf("no processes found using file %s", pathToFile)
	}

	return processes, nil
}
