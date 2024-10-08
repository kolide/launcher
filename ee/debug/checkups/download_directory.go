package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type downloadDirectory struct {
	status  Status
	summary string
	files   []fileInfo
}

func (c *downloadDirectory) Name() string {
	return "Kolide Downloads"
}

func (c *downloadDirectory) Run(_ context.Context, extraFH io.Writer) error {
	downloadDirs := getDownloadDirs()
	if len(downloadDirs) == 0 {
		c.status = Erroring
		c.summary = "No download directories found"
		return nil
	}

	for _, downloadDir := range downloadDirs {
		pattern := filepath.Join(downloadDir, "kolide-launcher*")
		matches, err := filepath.Glob(pattern)

		if err != nil {
			fmt.Fprintf(extraFH, "Error listing files in directory (%s): %s\n", downloadDir, err)
			continue
		}

		for _, match := range matches {
			if info, err := os.Stat(match); err == nil {
				c.files = append(c.files, fileInfo{
					Name:    match,
					ModTime: info.ModTime(),
				})
			}
		}
	}

	switch {
	case len(c.files) == 0:
		c.status = Informational
		c.summary = "No Kolide installers found in any user's download directory"
	default:
		c.status = Informational
		fileInfos := make([]string, len(c.files))
		for i, file := range c.files {
			fileName := filepath.Base(file.Name)
			modTime := file.ModTime.Format("Mon, 02 Jan 2006 15:04:05 MST")
			fileInfos[i] = fmt.Sprintf("%s (Modified: %s)", fileName, modTime)
		}
		installerList := strings.Join(fileInfos, ", ")
		c.summary = fmt.Sprintf("Found Kolide installer(s) across user download directories: %s", installerList)
	}

	if len(c.files) > 0 {
		fmt.Fprintln(extraFH, "Kolide installers found:")
		for _, file := range c.files {
			fmt.Fprintln(extraFH, file)
		}
	}

	return nil
}

func (c *downloadDirectory) ExtraFileName() string {
	return "kolide-installers-all-users.txt"
}

func (c *downloadDirectory) Status() Status {
	return c.status
}

func (c *downloadDirectory) Summary() string {
	return c.summary
}

func (c *downloadDirectory) Data() any {
	return c.files
}

func getDownloadDirs() []string {
	var userDirs []string
	var baseDir string

	if runtime.GOOS == "windows" {
		baseDir = "C:\\Users"
	} else if runtime.GOOS == "darwin" {
		baseDir = "/Users"
	} else {
		baseDir = "/home"
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		userDir := filepath.Join(baseDir, entry.Name(), "Downloads")
		if _, err := os.Stat(userDir); err == nil {
			userDirs = append(userDirs, userDir)
		}
	}

	return userDirs
}
