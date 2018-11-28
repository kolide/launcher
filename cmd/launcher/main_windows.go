// +build windows

package main

import (
	"os"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/logutil"
	"github.com/pkg/errors"

	"golang.org/x/sys/windows/svc"
)

const serviceName = "launcher"

func main() {
	var logger log.Logger
	logger = log.NewLogfmtLogger(os.Stderr) // temporary

	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		logutil.Fatal(logger, "err", errors.Wrap(err, "cannot determine if session is interactive"))
	}

	if !isIntSess {
		// run daemon
		return
	}

	// handle positional arg stuff

}
