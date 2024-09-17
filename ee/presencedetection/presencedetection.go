package presencedetection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/consoleuser"
)

type PresenceDetectionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type PresenceDetector struct {
	lastDetectionUTC time.Time
	mutext           sync.Mutex
}

func (pd *PresenceDetector) DetectForConsoleUser(reason string, detectionInterval time.Duration) (bool, error) {
	pd.mutext.Lock()
	defer pd.mutext.Unlock()

	// Check if the last detection was within the detection interval
	if time.Since(pd.lastDetectionUTC) < detectionInterval {
		return true, nil
	}

	executablePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("could not get executable path: %w", err)
	}

	consoleUserUids, err := consoleuser.CurrentUids(context.TODO())
	if err != nil {
		return false, fmt.Errorf("could not get console user: %w", err)
	}

	if len(consoleUserUids) == 0 {
		return false, errors.New("no console user found")
	}

	runningUserUid := consoleUserUids[0]

	// Ensure that we handle a non-root current user appropriately
	currentUser, err := user.Current()
	if err != nil {
		return false, fmt.Errorf("getting current user: %w", err)
	}

	runningUser, err := user.LookupId(runningUserUid)
	if err != nil || runningUser == nil {
		return false, fmt.Errorf("looking up user with uid %s: %w", runningUserUid, err)
	}

	cmd := exec.Command(executablePath, "detect-presence", reason) //nolint:forbidigo // We trust that the launcher executable path is correct, so we don't need to use allowedcmd

	// Update command so that we're prepending `launchctl asuser $UID sudo --preserve-env -u $runningUser` to the launcher desktop command.
	// We need to run with `launchctl asuser` in order to get the user context, which is required to be able to send notifications.
	// We need `sudo -u $runningUser` to set the UID on the command correctly -- necessary for, among other things, correctly observing
	// light vs dark mode.
	// We need --preserve-env for sudo in order to avoid clearing SOCKET_PATH, AUTHTOKEN, etc that are necessary for the desktop
	// process to run.
	cmd.Path = "/bin/launchctl"
	updatedCmdArgs := append([]string{"/bin/launchctl", "asuser", runningUserUid, "sudo", "--preserve-env", "-u", runningUser.Username}, cmd.Args...)
	cmd.Args = updatedCmdArgs

	if currentUser.Uid != "0" && currentUser.Uid != runningUserUid {
		// if the user is running for another user, we have an error because we can't set credentials
		return false, fmt.Errorf("current user %s is not root and does not match running user, can't start process for other user %s", currentUser.Uid, runningUserUid)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("could not run command: %w", err)
	}

	outStr := string(out)

	// get last line of outstr
	lastLine := ""
	for _, line := range strings.Split(outStr, "\n") {
		if line != "" {
			lastLine = line
		}
	}

	outDecoded, err := base64.StdEncoding.DecodeString(lastLine)
	if err != nil {
		return false, fmt.Errorf("could not decode output: %w", err)
	}

	response := PresenceDetectionResponse{}
	if err := json.Unmarshal(outDecoded, &response); err != nil {
		return false, fmt.Errorf("could not unmarshal response: %w", err)
	}

	if response.Success {
		pd.lastDetectionUTC = time.Now().UTC()
	}

	if response.Error != "" {
		return response.Success, errors.New(response.Error)
	}

	return response.Success, nil
}
