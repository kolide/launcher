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

// there is an edge case where the code below will return the previous logged in user
// (a new user has the console, but the last logged in user is returned)
// based on testing, this only occurs when:
//
// 1. an existing user logs in
// 2. the user uses "fast user switching" to log in as another user who has never logged in before
//    * fast user switching: (https://support.apple.com/guide/mac-help/switch-quickly-between-users-mchlp2439/mac)
// 3. even after the new user completes their first login flow, the last user is still marked as
//    kCGSSessionOnConsoleKey : TRUE and gets returned
// 4. after the new user logs out and logs back in, the new user is returned
//
// tested on M1 Monterey 12.6

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

		if kCGSSessionOnConsole == "TRUE" {
			uids = append(uids, kCGSSessionUserID)
		}

		kCGSSessionOnConsole = ""
		kCGSSessionUserID = ""
	}

	return uids, nil
}
