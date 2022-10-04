//go:build linux
// +build linux

package consoleuser

import (
	"fmt"
)

func CurrentUids(context context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
