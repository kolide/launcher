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

		// this is the name of the user at the login window
		// kCGSSessionOnConsoleKey can still show true for other users here
		if strings.Contains(line, "Name : loginwindow") {
			return nil, nil
		}

		const kCGSSessionUserIDKey = "kCGSSessionUserIDKey : "
		const kCGSSessionOnConsoleKey = "kCGSSessionOnConsoleKey : "

		switch {
		// got user id key
		case strings.Contains(line, kCGSSessionUserIDKey):
			kCGSSessionUserID = scutilVal(line, kCGSSessionUserIDKey)

		// got session key
		case strings.Contains(line, kCGSSessionOnConsoleKey):
			kCGSSessionOnConsole = scutilVal(line, kCGSSessionOnConsoleKey)

		default:
			continue
		}

		// if we don't have both values
		if kCGSSessionOnConsole == "" || kCGSSessionUserID == "" {
			continue
		}

		// have both values
		if kCGSSessionOnConsole == "TRUE" {
			uids = append(uids, kCGSSessionUserID)
		}

		// reset values
		kCGSSessionOnConsole = ""
		kCGSSessionUserID = ""
	}

	return uids, nil
}

func scutilVal(line, key string) string {
	parts := strings.Split(line, key)
	if len(parts) != 2 {
		return ""
	}

	return parts[1]
}
