package menu

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/go-kit/kit/log/level"
)

// actionTypes are named string identifiers
type actionType string

const (
	None        actionType = "" // Omitted action implies do nothing
	OpenURL                = "open-url"
	OpenWindow             = "open-window"
	Flare                  = "flare"
	RefreshMenu            = "refresh-menu"
)

// ActionData encapsulates what action should be performed when a menu item is invoked
type actionData struct {
	Type actionType `json:"type"`
	Data string     `json:"data,omitempty"`
}

// Action is an interface for performing actions in response to menu events
type Action interface {
	// Perform executes the action
	Perform(m *menu)
}

func (a actionData) Perform(m *menu) {
	var err error
	switch a.Type {
	case None:
		return
	case OpenURL:
		err = open(a.Data)
	case Flare:
		m.flareCommand(a.Data)
	case RefreshMenu:
		m.Build()
	default:
		level.Debug(m.logger).Log(
			"msg", "invalid action type",
			"type", a.Type)
		return
	}

	if err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform action",
			"type", a.Type,
			"data", a.Data,
			"err", err)
	}
}

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "/usr/bin/open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// flareCommand invokes the launcher flare executable with the appropriate env vars
func (m *menu) flareCommand(hostname string) (*exec.Cmd, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("error getting executable path: %w", err)
	}

	cmd := exec.Command(executablePath, "flare")
	cmd.Env = []string{
		fmt.Sprintf("HOSTNAME=%s", hostname),
	}

	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stderr pipe: %w", err)
	}

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stdout pipe: %w", err)
	}

	go func() {
		combined := io.MultiReader(stdErr, stdOut)
		scanner := bufio.NewScanner(combined)

		for scanner.Scan() {
			level.Info(m.logger).Log(
				"uid", os.Getuid(),
				"subprocess", "flare",
				"msg", scanner.Text(),
			)
		}
	}()

	return cmd, nil
}
