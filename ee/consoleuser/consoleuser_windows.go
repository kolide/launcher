//go:build windows
// +build windows

package consoleuser

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/carlpett/winlsa"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/execparsers/data_table"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

func CurrentUids(ctx context.Context) ([]string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

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
			return nil, fmt.Errorf("getting process owner uid (for pid %d): %w", explorerProc.Pid, err)
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

func CurrentUidsViaQuser(ctx context.Context) ([]string, error) {
	activeUsernames, err := currentUsernames(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting current usernames: %w", err)
	}

	usernameMap, err := usernameToSIDMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting map from username to SID: %w", err)
	}

	currentUids := make([]string, 0)
	for _, activeUsername := range activeUsernames {
		if sid, sidFound := usernameMap[activeUsername]; sidFound {
			currentUids = append(currentUids, sid)
		}
	}

	return currentUids, nil
}

func currentUsernames(ctx context.Context) ([]string, error) {
	queryUsersCmd, err := allowedcmd.Quser(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating quser cmd: %w", err)
	}

	usersRaw, err := queryUsersCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running quser: output `%s`: %w", string(usersRaw), err)
	}

	usersParser := data_table.NewParser()
	parsedUsers, err := usersParser.Parse(bytes.NewReader(usersRaw))
	if err != nil {
		return nil, fmt.Errorf("parsing quser output: %w", err)
	}
	parsedUsersList, ok := parsedUsers.([]map[string]string)
	if !ok {
		return nil, fmt.Errorf("unexpected return format %T from parsing quser output", parsedUsers)
	}

	activeUserList := make([]string, 0)
	for _, user := range parsedUsersList {
		if state, stateFound := user["STATE"]; !stateFound || state != "Active" {
			continue
		}

		// Found an active user
		if username, usernameFound := user["USERNAME"]; usernameFound {
			// Per https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/query-user:
			// A greater than (>) symbol is displayed before the current session. We want to remove this symbol if present.
			activeUserList = append(activeUserList, strings.ToLower(strings.TrimPrefix(username, ">")))
		}
	}

	return activeUserList, nil
}

func usernameToSIDMap(ctx context.Context) (map[string]string, error) {
	wmicCmd, err := allowedcmd.Wmic(ctx, "useraccount", "get", "name,sid")
	if err != nil {
		return nil, fmt.Errorf("creating wmic cmd: %w", err)
	}

	userListRaw, err := wmicCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running wmic useraccount get name,sid: output `%s`: %w", string(userListRaw), err)
	}

	userListParser := data_table.NewParser()
	parsedUsers, err := userListParser.Parse(bytes.NewReader(userListRaw))
	if err != nil {
		return nil, fmt.Errorf("parsing wmic output: %w", err)
	}
	parsedUsersList, ok := parsedUsers.([]map[string]string)
	if !ok {
		return nil, fmt.Errorf("unexpected return format %T from parsing wmic output", parsedUsers)
	}

	usernameMap := make(map[string]string)
	for _, user := range parsedUsersList {
		username, usernameFound := user["Name"]
		sid, sidFound := user["SID"]

		if !usernameFound || !sidFound {
			continue
		}
		usernameMap[strings.ToLower(username)] = sid
	}

	return usernameMap, nil
}

func CurrentUidsViaLsa(ctx context.Context) ([]string, error) {
	luids, err := winlsa.GetLogonSessions()
	if err != nil {
		return nil, fmt.Errorf("getting logon sessions: %w", err)
	}

	activeSids := make([]string, 0)
	for _, luid := range luids {
		sessionData, err := winlsa.GetLogonSessionData(&luid)
		if err != nil {
			return nil, fmt.Errorf("getting logon session data for LUID: %w", err)
		}

		// We get duplicates -- ignore those.
		if slices.Contains(activeSids, sessionData.Sid.String()) {
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
		activeSids = append(activeSids, sessionData.Sid.String())
	}

	return activeSids, nil
}
