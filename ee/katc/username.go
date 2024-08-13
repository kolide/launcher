package katc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strings"
)

var homeDirLocations = map[string][]string{
	"windows": {"/Users"},
	"darwin":  {"/Users"},
	"linux":   {"/home"},
}

// addUsernameFromFilePath is a dataProcessingStep that adds a `username` field to the row
// by checking the `sourcePath` for a user home directory prefix.
func addUsernameFromFilePath(_ context.Context, _ *slog.Logger, sourcePath string, row map[string][]byte) (map[string][]byte, error) {
	homeDirs, ok := homeDirLocations[runtime.GOOS]
	if !ok {
		return row, errors.New("cannot determine home directories")
	}

	for _, homeDir := range homeDirs {
		if !strings.HasPrefix(sourcePath, homeDir) {
			continue
		}

		// Trim the home directory and the leading path separator.
		// The next component of the path will then be the username.
		remainingPath := strings.TrimPrefix(strings.TrimPrefix(sourcePath, homeDir), string(os.PathSeparator))
		remainingPathComponents := strings.Split(remainingPath, string(os.PathSeparator))
		if len(remainingPathComponents) < 1 {
			continue
		}
		row["username"] = []byte(remainingPathComponents[0])
		return row, nil
	}

	return row, nil
}
