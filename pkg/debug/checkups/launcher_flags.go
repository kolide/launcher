package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/peterbourgon/ff/v3"
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
	fmt.Fprint(extraFh, fmt.Sprintf("flags file path: %s\n", configFilePath))

	stat, err := os.Stat(configFilePath)
	if err != nil {
		lf.summary = fmt.Sprintf("failed to stat %s: %s", configFilePath, err)
		return nil
	}

	fmt.Fprint(extraFh, fmt.Sprintf("flags file is %d bytes, and was last modified at %s\n", stat.Size(), stat.ModTime()))

	file, err := os.Open(configFilePath)
	if err != nil {
		lf.summary = fmt.Sprintf("failed to open %s: %s", configFilePath, err)
		return nil
	}

	fmt.Fprint(extraFh, "Parsed Flags (boolean flags appear as {KEY} TRUE):\n\n")

	if err := ff.PlainParser(file, func(name, value string) error {
		// print each flag and value to file handler
		fmt.Fprint(extraFh, fmt.Sprintf("%s %s\n", name, value))
		return nil
	}); err != nil {
		fmt.Fprint(extraFh, fmt.Sprintf("encountered error: %s", err))
		lf.summary = fmt.Sprintf("failed to parse flags: %s", err)
		return nil
	}

	lf.status = Passing
	lf.summary = "launcher flags exists and are parsable"
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
	if runtime.GOOS != "windows" {
		return "/private/etc/kolide-k2/launcher.flags"
	}

	// windows flags file path
	return filepath.Join(lf.k.RootDirectory(), "conf", "launcher.flags")
}
