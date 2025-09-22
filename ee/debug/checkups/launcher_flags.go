package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/launcher"
)

type launcherFlags struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (lf *launcherFlags) Name() string {
	return "Launcher Flags"
}

func (lf *launcherFlags) Run(_ context.Context, extraFh io.Writer) error {
	lf.status = Failing

	configFilePath := lf.flagsFilePath()
	fmt.Fprintf(extraFh, "flags file path: %s\n", configFilePath)

	stat, err := os.Stat(configFilePath)
	if err != nil {
		lf.summary = fmt.Sprintf("failed to stat %s: %s", configFilePath, err)
		return nil
	}

	fmt.Fprintf(extraFh, "flags file is %d bytes, and was last modified at %s\n", stat.Size(), stat.ModTime())

	file, err := os.Open(configFilePath)
	if err != nil {
		lf.summary = fmt.Sprintf("failed to open %s: %s", configFilePath, err)
		return nil
	}
	defer file.Close()

	fmt.Fprint(extraFh, "\nlauncher.flags contents:\n\n")

	if _, err := io.Copy(extraFh, file); err != nil {
		lf.summary = fmt.Sprintf("failed to copy flags file to file handler: %s", err)
		return nil
	}

	if _, err := launcher.ParseOptions("", []string{fmt.Sprintf("--config=%s", configFilePath)}); err != nil {
		lf.summary = fmt.Sprintf("failed to parse flags: %s", err)
		return nil
	}

	lf.status = Passing
	lf.summary = "launcher.flags exists and is parsable"
	return nil
}

func (lf *launcherFlags) Status() Status {
	return lf.status
}

func (lf *launcherFlags) Summary() string {
	return lf.summary
}

func (lf *launcherFlags) ExtraFileName() string {
	return "launcherFlags.log"
}

func (lf *launcherFlags) Data() any {
	return nil
}

func (lf *launcherFlags) flagsFilePath() string {
	identifier := launcher.DefaultLauncherIdentifier
	if lf.k.Identifier() != "" {
		identifier = lf.k.Identifier()
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(fmt.Sprintf(`C:\Program Files\Kolide\Launcher-%s\conf`, identifier), "launcher.flags")
	}

	// non-windows
	return fmt.Sprintf("/etc/%s/launcher.flags", identifier)
}
