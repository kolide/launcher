package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type Processes struct {
	kolideCount int
	data        []string // FIXME: this should be more structured
}

func (c *Processes) Name() string {
	return "Process Report"
}

func (c *Processes) Run(ctx context.Context, fullWriter io.Writer) error {
	if fullWriter == nil {
		return errors.New("missing io.Writer")
	}

	c.data = []string{}

	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting process list: %w", err)
	}

	for _, p := range ps {
		exe, _ := p.Exe()

		// full gets the full process logs. This lets us examine them for processes that might interfere.
		fmt.Fprintf(fullWriter, p.String())

		if strings.Contains(strings.ToLower(exe), "kolide") {
			c.kolideCount += 1
		}

		// FIXME: this should be more structured
		c.data = append(c.data, p.String())
	}

	return nil
}

func (c *Processes) ExtraFileName() string {
	return "process.txt"
}

func (c *Processes) Status() Status {
	if c.kolideCount >= 2 {
		return Passing
	}

	return Failing
}

func (c *Processes) Summary() string {
	return fmt.Sprintf("found %d kolide processes", c.kolideCount)
}

func (c *Processes) Data() any {
	return c.data
}
