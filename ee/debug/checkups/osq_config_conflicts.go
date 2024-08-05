package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// osqConfigConflictCheckup is a checkup intended to search for
// non-launcher (potentially conflicting) osquery installation config files.
// this is accomplished by checking for the presence of, and contents within
// any default osquery config directories for the target OS
type osqConfigConflictCheckup struct {
	status  Status
	summary string
	data    map[string]any
}

func (occ *osqConfigConflictCheckup) Data() any             { return occ.data }
func (occ *osqConfigConflictCheckup) ExtraFileName() string { return "" }
func (occ *osqConfigConflictCheckup) Name() string          { return "Osquery Conflicts" }
func (occ *osqConfigConflictCheckup) Status() Status        { return occ.status }
func (occ *osqConfigConflictCheckup) Summary() string       { return occ.summary }

func (occ *osqConfigConflictCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	occ.data = make(map[string]any)
	dirExists, dirNotEmpty, dirHasError := false, false, false
	var notableFileDirs []string
	switch runtime.GOOS {
	case "windows":
		notableFileDirs = []string{`C:\Program Files\osquery`}
	default:
		notableFileDirs = []string{"/var/osquery", "/etc/osquery"}
	}

	for _, dirname := range notableFileDirs {
		files, err := os.ReadDir(dirname)

		switch {
		case errors.Is(err, os.ErrNotExist):
			occ.data[dirname] = "not present"
		case err != nil:
			dirHasError = true
			occ.data[dirname] = err.Error()
		case len(files) == 0:
			dirExists = true
			occ.data[dirname] = "present, but empty"
		default:
			dirNotEmpty = true
			fileToLog := make([]string, len(files))

			for i, file := range files {
				fileToLog[i] = file.Name()
			}

			occ.data[dirname] = fmt.Sprintf("contains: %s", strings.Join(fileToLog, ", "))
		}
	}

	if dirNotEmpty {
		occ.status = Warning
		occ.summary = "At least one notable directory is present and non-empty"
	} else if dirHasError {
		occ.status = Erroring
		occ.summary = "At least one notable directory is present and could not be read"
	} else if dirExists {
		occ.status = Informational
		occ.summary = "At least one notable directory is present, but empty"
	} else {
		occ.status = Passing
		occ.summary = "No notable directories were detected"
	}

	return nil
}
