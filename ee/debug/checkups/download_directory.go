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

type DownloadDirectory struct {
	status  Status
	summary string
	files   []string
}

func (c *DownloadDirectory) Name() string {
	return "Download directory contents"
}

func (c *DownloadDirectory) Run(_ context.Context, extraFH io.Writer) error {
	downloadDir := getDownloadDir()
	if downloadDir == "" {
		return fmt.Errorf("no default download directory")
	}

	err := filepath.Walk(downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isKolideInstaller(info.Name()) {
			c.files = append(c.files, path)
		}
		return nil
	})

	switch {
	case os.IsNotExist(err):
		c.status = Warning
		c.summary = fmt.Sprintf("download directory (%s) not present", downloadDir)
	case err != nil:
		c.status = Erroring
		c.summary = fmt.Sprintf("error listing files in directory (%s): %s", downloadDir, err)
	case len(c.files) == 0:
		c.status = Warning
		c.summary = fmt.Sprintf("no Kolide installers found in directory (%s)", downloadDir)
	default:
		c.status = Passing
		fileNames := make([]string, len(c.files))
		for i, file := range c.files {
			fileNames[i] = filepath.Base(file)
		}
		installerList := strings.Join(fileNames, ", ")
		c.summary = fmt.Sprintf("Found Kolide installer(s) in directory (%s): %s", downloadDir, installerList)

	}

	if len(c.files) > 0 {
		fmt.Fprintln(extraFH, "Kolide installers found:")
		for _, file := range c.files {
			fmt.Fprintln(extraFH, file)
		}
	}

	return nil
}

func (c *DownloadDirectory) ExtraFileName() string {
	return "kolide-installers"
}

func (c *DownloadDirectory) Status() Status {
	return c.status
}

func (c *DownloadDirectory) Summary() string {
	return c.summary
}

func (c *DownloadDirectory) Data() any {
	return c.files
}

func getDownloadDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Downloads")
	case "linux":
		return filepath.Join(homeDir, "Downloads")
	case "windows":
		return filepath.Join(homeDir, "Downloads")
	}

	return ""
}

func isKolideInstaller(filename string) bool {
	lowerFilename := strings.ToLower(filename)
	switch runtime.GOOS {
	case "darwin":
		return strings.HasSuffix(lowerFilename, "kolide-launcher.pkg")
	case "linux":
		return strings.HasSuffix(lowerFilename, "kolide-launcher.rpm") ||
			strings.HasSuffix(lowerFilename, "kolide-launcher.deb") ||
			strings.HasSuffix(lowerFilename, "kolide-launcher.pacman")
	case "windows":
		return strings.HasSuffix(lowerFilename, "kolide-launcher.msi")
	}
	return false
}
