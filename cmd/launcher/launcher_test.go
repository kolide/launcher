package main

import (
	"context"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/kolide/launcher/v2/pkg/rungroup"
	"github.com/stretchr/testify/require"
)

// Test_runLauncher confirms that runLauncher can start up without error,
// and that it can shut down within our desired timeout.
func Test_runLauncher(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	// Set up opts
	testRootDir := t.TempDir()
	defaultOpts, err := launcher.ParseOptions("launcher", []string{
		"--root_directory", testRootDir,
	})
	require.NoError(t, err)

	// runLauncher, if able to start up without error, will run until stopped by
	// an autoupdate requiring reload, a sigterm, or a rungroup actor error.
	// So, start it up in the background.
	runLauncherErr := make(chan error)
	go func() {
		runLauncherErr <- runLauncher(ctx, cancel, multislogger.New(), multislogger.New(), defaultOpts)
	}()

	// launcher should run successfully, not immediately return an error.
	select {
	case err := <-runLauncherErr:
		t.Errorf("runLauncher did not start up successfully: returned %v", err)
	case <-time.After(rungroup.InterruptTimeout * 2):
		// launcher started up and stayed up
	}

	// Now, call cancel() to shut down runLauncher.
	cancel()
	select {
	case <-runLauncherErr:
		// launcher shut down successfully
	case <-time.After(rungroup.InterruptTimeout):
		t.Error("runLauncher did not return within interrupt timeout")
	}
}
