package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

type BinaryDirectory struct {
	status  Status
	summary string
}

func (c *BinaryDirectory) Name() string {
	return "Binary directory contents"
}

func (c *BinaryDirectory) Run(_ context.Context, extraFH io.Writer) error {
	bindir := getBinDir()
	if bindir == "" {
		return errors.New("No default bin directory")
	}

	// Note that we're recursing `/usr/local/kolide-k2` and not .../bin. So the counts may not be what
	// you expect. (But the flare output is better)
	filecount, err := recursiveDirectoryContents(extraFH, bindir)

	switch {
	case errors.Is(err, os.ErrNotExist):
		c.status = Failing
		c.summary = fmt.Sprintf("binary directory (%s) not present", bindir)
	case err != nil:
		c.status = Erroring
		c.summary = fmt.Sprintf("listing files in binary directory (%s): %s", bindir, err)
	case filecount == 0:
		c.status = Warning
		c.summary = fmt.Sprintf("binary directory (%s) empty", bindir)
	default:
		c.status = Passing
		c.summary = fmt.Sprintf("binary directory (%s) contains %d files", bindir, filecount)
	}
	return nil
}

func (c *BinaryDirectory) ExtraFileName() string {
	return "file-list"
}

func (c *BinaryDirectory) Status() Status {
	return c.status
}

func (c *BinaryDirectory) Summary() string {
	return c.summary
}

func (c *BinaryDirectory) Data() any {
	return nil
}

// getBinDir returns the platform default binary directory. It should probably get folded into flags, but I'm not
// quite sure how yet.
func getBinDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/usr/local/kolide-k2"
	case "linux":
		return "/usr/local/kolide-k2"
	case "windows":
		return "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin"
	}

	return ""
}
