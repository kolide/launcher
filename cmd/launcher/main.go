// +build !windows

package main

import (
	"os"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/logutil"
	"github.com/pkg/errors"
)

func main() {
	var logger log.Logger
	logger = log.NewJSONLogger(os.Stderr) // only used until options are parsed.

	// if the launcher is being ran with a positional argument, handle that
	// argument. If a known positional argument is not supplied, fall-back to
	// running an osquery instance.
	if isSubCommand() {
		if err := runSubcommands(); err != nil {
			logutil.Fatal(logger, "err", errors.Wrap(err, "run with positional args"))
		}
	}

	err := runLauncher()
	if err != nil {
		logutil.Fatal(logger, err, "run launcher")
	}
}
