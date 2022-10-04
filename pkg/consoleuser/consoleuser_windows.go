//go:build windows
// +build windows

package consoleuser

import (
	"fmt"
)

func CurrentUids(context context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
