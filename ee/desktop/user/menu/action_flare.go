package menu

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/go-kit/kit/log/level"

	"github.com/mitchellh/go-homedir"
)

// Performs the launcher flare action
type actionFlare struct {
	URL string `json:"url"`
}

func (a actionFlare) Perform(m *menu) {
	cmd, err := m.flareCommand(m.hostname)
	if err != nil {
		level.Error(m.logger).Log("msg", "failed to exec launcher flare", "err", err)
	}
	cmd.Start()
}

// flareCommand invokes the launcher flare executable with the appropriate env vars
func (m *menu) flareCommand(hostname string) (*exec.Cmd, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("error getting executable path: %w", err)
	}

	desktopDir, err := homedir.Expand("~/Desktop")
	cmd := exec.Command(executablePath, "flare")
	cmd.Env = []string{
		fmt.Sprintf("HOSTNAME=%s", hostname),
		fmt.Sprintf("KOLIDE_LAUNCHER_TAR_DIR_PATH=%s", desktopDir),
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
