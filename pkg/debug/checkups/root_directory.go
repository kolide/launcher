package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kolide/launcher/pkg/agent/types"
)

type RootDirectory struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *RootDirectory) Name() string {
	return "Root directory contents"
}

func (c *RootDirectory) Run(_ context.Context, fullFH io.Writer) error {
	files, err := os.ReadDir(c.k.RootDirectory())
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.status = Failing
		c.summary = fmt.Sprintf("root directory (%s) not present", c.k.RootDirectory())
	case err != nil:
		c.status = Erroring
		c.summary = fmt.Sprintf("listing files in root directory (%s): %s", c.k.RootDirectory(), err)
	case len(files) == 0:
		c.status = Warning
		c.summary = fmt.Sprintf("root directory (%s) empty", c.k.RootDirectory())
	default:
		c.status = Passing
		c.summary = fmt.Sprintf("root directory (%s) contains %d files", c.k.RootDirectory(), len(files))

		fmt.Fprintf(fullFH, "%s\n", files)
	}
	return nil
}

func (c *RootDirectory) ExtraFileName() string {
	return "file-list"
}

func (c *RootDirectory) Status() Status {
	return c.status
}

func (c *RootDirectory) Summary() string {
	return c.summary
}

func (c *RootDirectory) Data() any {
	return nil
}
