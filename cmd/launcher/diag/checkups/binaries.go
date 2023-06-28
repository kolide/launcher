package checkups

import (
	"fmt"
	"io"
	"path/filepath"
)

type Binaries struct {
	Filepaths []string
}

func (c *Binaries) Name() string {
	return "Launcher application"
}

func (c *Binaries) Run(short io.Writer) (string, error) {
	return checkupAppBinaries(short, c.Filepaths)
}

type launcherFile struct {
	name  string
	found bool
}

func checkupAppBinaries(short io.Writer, filepaths []string) (string, error) {
	importantFiles := []*launcherFile{
		{
			name: windowsAddExe("launcher"),
		},
	}

	return checkupFilesPresent(short, filepaths, importantFiles)
}

func checkupFilesPresent(short io.Writer, filepaths []string, importantFiles []*launcherFile) (string, error) {
	if filepaths != nil && len(filepaths) > 0 {
		for _, fp := range filepaths {
			for _, f := range importantFiles {
				if filepath.Base(fp) == f.name {
					f.found = true
				}
			}
		}
	}

	var failures int
	for _, f := range importantFiles {
		if f.found {
			pass(short, f.name)
		} else {
			fail(short, f.name)
			failures = failures + 1
		}
	}

	if failures == 0 {
		return "Files found", nil
	}

	return "", fmt.Errorf("%d files not found", failures)
}
