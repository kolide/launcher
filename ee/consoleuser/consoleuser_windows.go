//go:build windows
// +build windows

package consoleuser

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"

	"github.com/shirou/gopsutil/process"
)

func CurrentUids(ctx context.Context) ([]string, error) {
	explorerProcs, err := explorerProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting explorer processes: %w", err)
	}

	// unclear if windows will ever have more than one explorer process for a single user
	// guard against this by forcing uniqueness

	// first store uids in a map to prevent duplicates
	// most of the time it will be just 1 user, so start map at 1
	uidsMap := make(map[string]struct{}, 1)

	for _, explorerProc := range explorerProcs {
		uid, err := processOwnerUid(ctx, explorerProc)
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

func ExplorerProcess(ctx context.Context, uid string) (*process.Process, error) {
	explorerProcs, err := explorerProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting explorer processes: %w", err)
	}

	for _, proc := range explorerProcs {
		procOwnerUid, err := processOwnerUid(ctx, proc)
		if err != nil {
			return nil, fmt.Errorf("getting process owner uid: %w", err)
		}

		if uid == procOwnerUid {
			return proc, nil
		}
	}

	return nil, nil
}

// explorerProcesses returns a list of explorer processes whose
// filepath base is "explorer.exe".
func explorerProcesses(ctx context.Context) ([]*process.Process, error) {
	var explorerProcs []*process.Process

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

func processOwnerUid(ctx context.Context, proc *process.Process) (string, error) {
	username, err := proc.UsernameWithContext(ctx)
	if err != nil {
		return "", fmt.Errorf("getting process username: %w", err)
	}

	user, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("looking up user: %w", err)
	}

	return user.Uid, nil
}
