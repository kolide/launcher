package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kolide/launcher/pkg/agent/types"
)

var notableFileDirs = []string{"/var/osquery", "/etc/osquery"}

type filesCheckup struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]string
}

func (fc *filesCheckup) Data() any             { return fc.data }
func (fc *filesCheckup) ExtraFileName() string { return "" }
func (fc *filesCheckup) Name() string          { return "Notable Files" }
func (fc *filesCheckup) Status() Status        { return fc.status }
func (fc *filesCheckup) Summary() string       { return fc.summary }

func (fc *filesCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	fc.data = make(map[string]string)
	dirExists, dirNotEmpty, dirHasError := false, false, false

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

	fc.status = Informational
	if dirNotEmpty {
		fc.summary = "At least one notable directory is present and non-empty"
	} else if dirHasError {
		fc.summary = "At least one notable directory is present and could not be read"
	} else if dirExists {
		fc.summary = "At least one notable directory is present, but empty"
	} else {
		fc.summary = "No notable directories were detected"
	}

	return nil
}
