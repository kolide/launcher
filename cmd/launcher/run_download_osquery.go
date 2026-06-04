package main

import (
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
)

// runDownloadOsquery downloads osqueryd to the provided path. It's meant for use in our CI
// pipeline. It is retained as a thin wrapper around the more general `download` subcommand so
// existing callers of `download-osquery` (CI, packaging) keep working; new callers should
// prefer `download --binary=osqueryd`.
func runDownloadOsquery(slogger *multislogger.MultiSlogger, args []string) error {
	return runDownload(slogger, append([]string{"--binary=osqueryd"}, args...))
}
