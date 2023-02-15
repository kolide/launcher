//go:build darwin
// +build darwin

package consoleuser

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// example scutil output
// ❯  scutil <<< "show State:/Users/ConsoleUser"
// <dictionary> {
//   GID : 20
//   Name : jandoe
//   SessionInfo : <array> {
//     0 : <dictionary> {
//       kCGSSessionAuditIDKey : 100099
//       kCGSSessionGroupIDKey : 20
//       kCGSSessionIDKey : 257
//       kCGSSessionLoginwindowSafeLogin : FALSE
//       kCGSSessionOnConsoleKey : TRUE
//       kCGSSessionSystemSafeBoot : FALSE
//       kCGSSessionUserIDKey : 501
//       kCGSSessionUserNameKey : jandoe
//       kCGSessionLoginDoneKey : TRUE
//       kCGSessionLongUserNameKey : Janice Doe
//       kSCSecuritySessionID : 100099
//     }
//     1 : <dictionary> {
//       kCGSSessionAuditIDKey : 100999
//       kCGSSessionGroupIDKey : 20
//       kCGSSessionIDKey : 265
//       kCGSSessionLoginwindowSafeLogin : FALSE
//       kCGSSessionOnConsoleKey : FALSE
//       kCGSSessionSystemSafeBoot : FALSE
//       kCGSSessionUserIDKey : 502
//       kCGSSessionUserNameKey : jrdoe
//       kCGSessionLoginDoneKey : TRUE
//       kCGSessionLongUserNameKey : Doe Junior
//       kSCSecuritySessionID : 100999
//     }
//   }
//   UID : 501
// }
//
// looking at the output above, here some of the important keys
//
// 1. kCGSSessionOnConsoleKey : < TRUE | FALSE >
//    if a new desktop icon is created it will appear on the desktop
//    of the user that has kCGSSessionOnConsoleKey : TRUE
//
// 2. kCGSSessionUserIDKey : < uid >
//
// 3. UID : < uid >
//    this is the uid of the user that is currently logged in
//
//    a mismatch between the outer UID and kCGSSessionUserIDKey indicates
//    that a new user was fast switched to and is going through
//    or has completed the first time login flow
//
//    we cannot detect if a user has completed the flow or not and
//    if we were to return a user in the process of the first time
//    login flow and a downstream process created an icon for that
//    user, it would appear on the last logged in user's console

// there is an edge case where the code below will return no uids when a user
// is in the process of or has just completed the first time login flow
// after being "fast switched" to
//
// this occurs when:
//
// 1. an existing user logs in
// 2. the user uses "fast user switching" to log in as another user who has never logged in before
//    * fast user switching: (https://support.apple.com/guide/mac-help/switch-quickly-between-users-mchlp2439/mac)
// 3. even after the new user completes their first login flow, the last user is still marked as
//    kCGSSessionOnConsoleKey : TRUE, which doesn't match the outer UID, so we continue to return no uids
// 4. after the new user logs out and logs back in, the new user is returned
//
// tested on M1 Monterey 12.6

const (
	// minConsoleUserUid is the minimum UID for human console users
	minConsoleUserUid = 501
)

func CurrentUids(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "scutil")
	cmd.Stdin = strings.NewReader("show State:/Users/ConsoleUser")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("executing scutil cmd: %w", err)
	}

	var uids []string

	kCGSSessionOnConsole := ""
	kCGSSessionUserID := ""
	lastkCGSSessionUserID := ""
	outerUid := ""

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch {
		// at login window
		case key == "Name" && val == "loginwindow":
			return nil, nil

		// this is the "outer uid" reported by scutil
		case key == "UID":
			outerUid = val

		case key == "kCGSSessionOnConsoleKey":
			kCGSSessionOnConsole = val

		case key == "kCGSSessionUserIDKey":
			kCGSSessionUserID = val

		default:
			continue
		}

		if kCGSSessionOnConsole == "" || kCGSSessionUserID == "" {
			continue
		}

		// We only care about human console users (UID 501 or greater)
		uidInt, err := strconv.Atoi(kCGSSessionUserID)
		if err != nil || uidInt < minConsoleUserUid {
			continue
		}

		if kCGSSessionOnConsole == "TRUE" {
			uids = append(uids, kCGSSessionUserID)
			lastkCGSSessionUserID = kCGSSessionUserID
		}

		kCGSSessionOnConsole = ""
		kCGSSessionUserID = ""

		// this is the edge case where scutil gives a mismatch between the UID returned
		// and the user that has kCGSSessionOnConsoleKey : TRUE
		// this occurs when a user goes through the login flow for the first time
		// after being "fast switched" to
		if outerUid != "" && lastkCGSSessionUserID != "" && lastkCGSSessionUserID != outerUid {
			return nil, nil
		}
	}

	return uids, nil
}
