package main

import (
	"context"

	"github.com/kolide/kit/logutil"
)

func runLoop(args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logutil.NewServerLogger(true)

	return runLauncher(ctx, cancel, args, logger)

}
