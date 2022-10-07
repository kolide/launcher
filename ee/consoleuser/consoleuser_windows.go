//go:build windows
// +build windows

package consoleuser

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/process"
)

func CurrentUids(context context.Context) ([]string, error) {
	explorerProcs, err := explorerProcesses()
	if err != nil {
		return nil, fmt.Errorf("getting explorer processes: %w", err)
	}

	// unclear if windows will ever have more than one explorer process for a single user
	// guard against this by forcing uniqueness

	// first store uids in a map to prevent duplicates
	// most of the time it will be just 1 user, so start map at 1
	uidsMap := make(map[string]struct{}, 1)

	for _, explorerProc := range explorerProcs {
		uid, err := processOwnerUid(explorerProc)
		if err != nil {
			return nil, fmt.Errorf("getting process owner uid: %w", err)
		}
		uidsMap[uid] = struct{}{}
	}

	// convert map keys to slice
	uids := make([]string, len(uidsMap))
	uidCount := 0
	for uid := range uidsMap {
		uids[uidCount] = uid
		uidCount++
	}

	return uids, nil
}

// explorerProcesses returns a list of explorer processes whose
// filepath base is "explorer.exe".
func explorerProcesses() ([]*process.Process, error) {
	var explorerProcs []*process.Process

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting processes: %w", err)
	}

	for _, proc := range procs {
		exe, err := proc.ExeWithContext(ctx)
		if err != nil {
			continue
		}

		if filepath.Base(exe) == "explorer.exe" {
			explorerProcs = append(explorerProcs, proc)
		}
	}

	return explorerProcs, nil
}

func processOwnerUid(proc *process.Process) (string, error) {
	username, err := proc.Username()
	if err != nil {
		return "", fmt.Errorf("getting process username: %w", err)
	}

	user, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("looking up user: %w", err)
	}

	return user.Uid, nil
}
