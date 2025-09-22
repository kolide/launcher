package checkups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

type Processes struct {
	kolideCount         int
	kolideRunningAsRoot bool
	data                map[string]any
}

func (c *Processes) Name() string {
	return "Process Report"
}

func (c *Processes) Run(ctx context.Context, fullWriter io.Writer) error {
	if fullWriter == nil {
		return errors.New("missing io.Writer")
	}

	c.data = make(map[string]any)

	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting process list: %w", err)
	}

	jsonWriter := json.NewEncoder(fullWriter)

	for _, p := range ps {
		// This doesn't feel great yet. I'm not sure what data we need, and some of the gopsutil stuff has
		// weird extraneous exec routines. So this is a starting point.
		pMap := map[string]any{
			"pid":         p.Pid,
			"exe":         naIfError(p.ExeWithContext(ctx)),
			"cmdline":     naIfError(p.CmdlineSliceWithContext(ctx)),
			"create_time": naIfError(p.CreateTimeWithContext(ctx)),
			"ppid":        naIfError(p.PpidWithContext(ctx)),
			"mem_info":    naIfError(p.MemoryInfoWithContext(ctx)),
			"mem_info_ex": naIfError(p.MemoryInfoExWithContext(ctx)),
			"cpu_times":   naIfError(p.TimesWithContext(ctx)),
			"status":      naIfError(p.StatusWithContext(ctx)),
			"uids":        naIfError(p.UidsWithContext(ctx)),
			"username":    naIfError(p.UsernameWithContext(ctx)),
		}
		_ = jsonWriter.Encode(pMap)

		exe, _ := p.Exe()
		if strings.Contains(strings.ToLower(exe), "kolide") {
			c.kolideCount += 1

			// Grab ENV vars -- available on windows/linux but not darwin;
			// included primarily for troubleshooting browser opening issues
			// on linux
			pMap["env"] = naIfError(p.EnvironWithContext(ctx))

			c.data[fmt.Sprintf("%d", p.Pid)] = pMap

			if !c.kolideRunningAsRoot {
				username := pMap["username"].(string)
				if username == "root" || username == "NT AUTHORITY\\SYSTEM" {
					c.kolideRunningAsRoot = true
				}
			}
		}
	}

	return nil
}

func (c *Processes) ExtraFileName() string {
	return "process.json"
}

func (c *Processes) Status() Status {
	if c.kolideCount >= 2 && c.kolideRunningAsRoot {
		return Passing
	}

	return Failing
}

func (c *Processes) Summary() string {
	if c.kolideRunningAsRoot {
		return fmt.Sprintf("found %d kolide processes, at least one running as root or system", c.kolideCount)
	}

	return fmt.Sprintf("found %d kolide processes, none running as root or system", c.kolideCount)
}

func (c *Processes) Data() any {
	return c.data
}

func naIfError(val any, err error) any {

	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	return val
}
