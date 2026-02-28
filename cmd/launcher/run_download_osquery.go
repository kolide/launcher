package main

import (
	"github.com/kolide/launcher/pkg/log/multislogger"
)

// runDownloadOsquery downloads the stable osquery to the provided path. It's meant for use in our CI pipeline.
// This is a legacy wrapper around runDownload for backward compatibility.
func runDownloadOsquery(slogger *multislogger.MultiSlogger, args []string) error {
	// Prepend "osqueryd" so runDownload receives it as the binary argument
	return runDownload(slogger, append([]string{"osqueryd"}, args...))
}
