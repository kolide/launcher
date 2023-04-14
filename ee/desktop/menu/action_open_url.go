package menu

import (
	"os/exec"
	"runtime"
	"syscall"

	"github.com/go-kit/kit/log/level"
)

// Performs the OpenURL action
type actionOpenURL struct {
	URL string `json:"url"`
}

func (a actionOpenURL) Perform(m *menu) {
	if err := open(a.URL); err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform action",
			"URL", a.URL,
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
		args = []string{"/C", "start"}
	case "darwin":
		cmd = "/usr/bin/open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)

	command := exec.Command(cmd, args...)
	if runtime.GOOS == "windows" {
		// https://stackoverflow.com/questions/42500570/how-to-hide-command-prompt-window-when-using-exec-in-golang
		// Otherwise the cmd window will appear briefly
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	return command.Start()
}
