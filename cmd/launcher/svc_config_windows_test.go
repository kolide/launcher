//go:build windows
// +build windows

package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_checkRootDirACLs(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	var logBytes threadsafebuffer.ThreadSafeBuffer

	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// run the check once, expecting that we will correctly work all the way through
	// and log that we've updated the ACLs for our new directory
	checkRootDirACLs(slogger, rootDir)
	require.Contains(t, logBytes.String(), "updated ACLs for root directory")

	// now clear the log, and rerun. if the previous run did what it was supposed to,
	// and our check-before-write logic works correctly, we should detect the ACL we
	// just added and exit early
	io.Copy(io.Discard, &logBytes)
	checkRootDirACLs(slogger, rootDir)
	require.NotContains(t, logBytes.String(), "updated ACLs for root directory")
	require.Contains(t, logBytes.String(), "root directory already had proper DACL permissions set, skipping")
}
