package checkups

import (
	"io"
)

type RootDirectory struct {
	Filepaths []string
}

func (c *RootDirectory) Name() string {
	return "Root directory contents"
}

func (c *RootDirectory) Run(short io.Writer) (string, error) {
	return checkupRootDir(short, c.Filepaths)
}

// checkupRootDir tests for the presence of important files in the launcher root directory
func checkupRootDir(short io.Writer, filepaths []string) (string, error) {
	importantFiles := []*launcherFile{
		{
			name: "debug.json",
		},
		{
			name: "launcher.db",
		},
		{
			name: "osquery.db",
		},
	}

	return checkupFilesPresent(short, filepaths, importantFiles)
}
