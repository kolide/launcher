//go:build windows
// +build windows

package consoleuser

import (
	"context"
	"fmt"
	"path/filepath"

	winlsa "github.com/kolide/go-winlsa"
	"github.com/kolide/launcher/ee/observability"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

// CurrentUids returns actual SIDs, but we've historically used usernames rather than SIDs on Windows.
// It is a trivial change to swap from sessionData.Sid.String() to sessionData.UserName here.
func CurrentUids(ctx context.Context) ([]string, error) {
	luids, err := winlsa.GetLogonSessions()
	if err != nil {
		return nil, fmt.Errorf("getting logon sessions: %w", err)
	}

	activeSids := make(map[string]any)
	for _, luid := range luids {
		sessionData, err := winlsa.GetLogonSessionData(&luid)
		if err != nil {
			return nil, fmt.Errorf("getting logon session data for LUID: %w", err)
		}

		if sessionData.Sid == nil {
			continue
		}

		// We get duplicates -- ignore those.
		if _, alreadyFound := activeSids[sessionData.Sid.String()]; alreadyFound {
			continue
		}

		// Only look at sessions associated with users. We can filter first by interactive-type logons,
		// to avoid extra syscalls.
		if sessionData.LogonType != winlsa.LogonTypeInteractive && sessionData.LogonType != winlsa.LogonTypeRemoteInteractive {
			continue
		}

		// We are left with a couple other non-user sessions. Check the account type now (requires a syscall).
		_, _, accountType, err := sessionData.Sid.LookupAccount("")
		if err != nil {
			return nil, fmt.Errorf("getting account type for LUID: %w", err)
		}
		if accountType != windows.SidTypeUser {
			continue
		}

		// We've got a real user -- add them to our list.
		activeSids[sessionData.Sid.String()] = struct{}{}
	}

	activeSidsList := make([]string, len(activeSids))
	i := 0
	for sid := range activeSids {
		activeSidsList[i] = sid
		i += 1
	}

	return activeSidsList, nil
}

func ExplorerProcess(ctx context.Context, uid string) (*process.Process, error) {
	ctx, span := observability.StartSpan(ctx, "uid", uid)
	defer span.End()

	explorerProcs, err := explorerProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting explorer processes: %w", err)
	}

	for _, proc := range explorerProcs {
		procOwnerUid, err := processOwnerUid(ctx, proc)
		if err != nil {
			return nil, fmt.Errorf("getting explorer process owner uid (for pid %d): %w", proc.Pid, err)
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
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

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
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	username, err := proc.UsernameWithContext(ctx)
	if err != nil {
		return "", fmt.Errorf("getting process username (for pid %d): %w", proc.Pid, err)
	}

	// Looking up the proper UID (which on Windows, is a SID) seems to be problematic and
	// can fail for reasons we don't quite understand. We just need something to uniquely
	// identify the user, so on Windows we use the username instead of numeric UID.
	return username, nil
}
