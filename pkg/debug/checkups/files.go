package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/kolide/launcher/pkg/agent/types"
)

type filesCheckup struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]any
}

func (fc *filesCheckup) Data() map[string]any  { return fc.data }
func (fc *filesCheckup) ExtraFileName() string { return "" }
func (fc *filesCheckup) Name() string          { return "Notable Files" }
func (fc *filesCheckup) Status() Status        { return fc.status }
func (fc *filesCheckup) Summary() string       { return fc.summary }

func (fc *filesCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	fc.data = make(map[string]any)
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
			fc.data[dirname] = "not present"
		case err != nil:
			dirHasError = true
			fc.data[dirname] = err.Error()
		case len(files) == 0:
			dirExists = true
			fc.data[dirname] = "present, but empty"
		default:
			dirNotEmpty = true
			fileToLog := make([]string, len(files))

			for i, file := range files {
				fileToLog[i] = file.Name()
			}

			fc.data[dirname] = fmt.Sprintf("contains: %s", strings.Join(fileToLog, ", "))
		}
	}

	if dirNotEmpty {
		fc.status = Failing
		fc.summary = "At least one notable directory is present and non-empty"
	} else if dirHasError {
		fc.status = Erroring
		fc.summary = "At least one notable directory is present and could not be read"
	} else if dirExists {
		fc.status = Warning
		fc.summary = "At least one notable directory is present, but empty"
	} else {
		fc.status = Passing
		fc.summary = "No notable directories were detected"
	}

	return nil
}
