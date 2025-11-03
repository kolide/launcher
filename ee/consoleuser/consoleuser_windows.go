//go:build windows
// +build windows

package consoleuser

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	winlsa "github.com/kolide/go-winlsa"
	"github.com/kolide/launcher/ee/observability"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

var (
	// knownInvalidUsernamesMap tracks usernames that we are repeatedly unable to find explorer.exe processes for,
	// indicating that we should not include them in the return for `CurrentUids`.
	knownInvalidUsernamesMap     = make(map[string]struct{})
	knownInvalidUsernamesMapLock = &sync.Mutex{}

	// potentialInvalidUsernamesMap tracks usernames that we are currently unable to find explorer.exe processes for.
	// If a username accrues too many invalid lookups in potentialInvalidUsernamesMap, it will move to knownInvalidUsernamesMap.
	potentialInvalidUsernamesMap     = make(map[string][]int64) // maps username to timestamps where we failed to find explorer.exe for that username
	potentialInvalidUsernamesMapLock = &sync.Mutex{}
)

const (
	invalidUsernameLookupWindowSeconds = 60
	maxUsernameLookupFailureCount      = 3
)

// CurrentUids is able to actual SIDs, but we've historically used usernames rather than SIDs on Windows.
// Therefore, we continue to return the usernames here. If we change our mind, it is a trivial change to swap
// from sessionData.UserName to sessionData.Sid.String() here.
func CurrentUids(ctx context.Context) ([]string, error) {
	luids, err := winlsa.GetLogonSessions()
	if err != nil {
		return nil, fmt.Errorf("getting logon sessions: %w", err)
	}

	activeUsernames := make(map[string]any)
	for _, luid := range luids {
		sessionData, err := winlsa.GetLogonSessionData(&luid)
		if err != nil {
			return nil, fmt.Errorf("getting logon session data for LUID: %w", err)
		}

		if sessionData.Sid == nil {
			continue
		}

		// We get duplicates -- ignore those.
		if _, alreadyFound := activeUsernames[usernameFromSessionData(sessionData)]; alreadyFound {
			continue
		}

		// Only look at sessions associated with users. We can filter first by interactive-type logons,
		// to avoid extra syscalls.
		if sessionData.LogonType != winlsa.LogonTypeInteractive && sessionData.LogonType != winlsa.LogonTypeRemoteInteractive {
			continue
		}

		// Check for a couple well-known accounts that we know we don't want to create desktop processes for.
		sessionUsername := usernameFromSessionData(sessionData)
		if shouldIgnoreSession(sessionData, sessionUsername) {
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
		activeUsernames[sessionUsername] = struct{}{}
	}

	activeUsernameList := slices.Collect(maps.Keys(activeUsernames))

	return activeUsernameList, nil
}

// shouldIgnoreSession will check the given session data's username and logon domain
// to see whether it's an account that we know we should not count as a current console user.
// We check for:
// 1. Okta Verify's user account (https://support.okta.com/help/s/article/Why-is-an-OVService-Account-Created-on-Windows-When-Installing-Okta-Verify?language=en_US)
// 2. The WsiAccount (https://learn.microsoft.com/en-us/windows/security/identity-protection/web-sign-in/?tabs=intune)
// 3. Any Desktop Windows Manager users
// 4. Any defaultuser0 that should have been cleaned up by Windows already but wasn't
// 5. Any account that we've repeatedly failed to find explorer.exe processes for
func shouldIgnoreSession(sessionData *winlsa.LogonSessionData, sessionUsername string) bool {
	// First, check for known account types
	if strings.HasPrefix(sessionData.UserName, "OVService-") ||
		strings.HasPrefix(sessionData.UserName, "OVSvc") ||
		sessionData.UserName == "WsiAccount" ||
		(sessionData.LogonDomain == "Window Manager" && strings.HasPrefix(sessionData.UserName, "DWM-")) ||
		strings.EqualFold(sessionData.UserName, "defaultuser0") {
		return true
	}
	// Now, check for account types that we've flagged as missing explorer.exe processes
	knownInvalidUsernamesMapLock.Lock()
	defer knownInvalidUsernamesMapLock.Unlock()
	if _, found := knownInvalidUsernamesMap[sessionUsername]; found {
		return true
	}

	return false
}

// usernameFromSessionData constructs a username in the format compatible with e.g.
// ExplorerProcess.
func usernameFromSessionData(sessionData *winlsa.LogonSessionData) string {
	return fmt.Sprintf("%s\\%s", sessionData.LogonDomain, sessionData.UserName)
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

		if strings.EqualFold(uid, procOwnerUid) {
			return proc, nil
		}
	}

	// We didn't find an explorer.exe process for the uid, so it's potentially an invalid username.
	updateInvalidUsernameMaps(uid, invalidUsernameLookupWindowSeconds)

	return nil, nil
}

func updateInvalidUsernameMaps(username string, windowSeconds int64) {
	// First, save the current timestamp to our potentialInvalidUsernamesMap.
	lookupTimestamp := time.Now().Unix()
	potentialInvalidUsernamesMapLock.Lock()
	defer potentialInvalidUsernamesMapLock.Unlock()
	if _, found := potentialInvalidUsernamesMap[username]; !found {
		// First failure -- add it to the map and return
		potentialInvalidUsernamesMap[username] = []int64{lookupTimestamp}
		return
	} else {
		potentialInvalidUsernamesMap[username] = append(potentialInvalidUsernamesMap[username], lookupTimestamp)
	}

	// We know we've seen this username before. Check to see if we have maxUsernameLookupFailureCount failures
	// within the lookup window.
	lookupWindowStart := lookupTimestamp - windowSeconds
	failureCount := 0
	lastTimestampOutsideWindowIdx := -1
	for i, ts := range potentialInvalidUsernamesMap[username] {
		if ts >= lookupWindowStart {
			failureCount += 1
			continue
		}
		lastTimestampOutsideWindowIdx = i
	}
	// Remove old timestamps outside of window, if any
	if lastTimestampOutsideWindowIdx > -1 {
		potentialInvalidUsernamesMap[username] = potentialInvalidUsernamesMap[username][lastTimestampOutsideWindowIdx+1:]
	}

	// Too many failures -- move to knownInvalidUsernamesMap.
	if failureCount >= maxUsernameLookupFailureCount {
		knownInvalidUsernamesMapLock.Lock()
		knownInvalidUsernamesMap[username] = struct{}{}
		knownInvalidUsernamesMapLock.Unlock()

		delete(potentialInvalidUsernamesMap, username)
	}
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
