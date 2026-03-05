package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/kolide/launcher/v2/pkg/packaging"
)

// runDownloadOsquery downloads the stable osquery to the provided path. It's meant for use in our CI pipeline.
// This is a legacy wrapper around runDownload for backward compatibility.
func runDownloadOsquery(slogger *multislogger.MultiSlogger, args []string) error {
	return runDownload(slogger, append([]string{"--target=osqueryd"}, args...))
}
