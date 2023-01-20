package menu

import (
	"os/exec"
	"runtime"

	"github.com/go-kit/kit/log/level"
)

// Performs the OpenURL action
type actionOpenURL struct {
	URL string `json:"url"`
}

func (a actionOpenURL) Perform(m *menu, parser textParser) {
	url, err := parser.parse(a.URL)
	if err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to parse URL",
			"URL", a.URL,
			"err", err)
		return
	}

	if err := open(url); err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform action",
			"URL", url,
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
