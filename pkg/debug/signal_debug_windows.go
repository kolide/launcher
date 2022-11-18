//go:build windows
// +build windows

package debug

import (
	"github.com/go-kit/kit/log"
)

func AttachDebugHandler(addrPath string, logger log.Logger) {
	// TODO: noop for now
}
