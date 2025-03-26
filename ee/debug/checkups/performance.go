package checkups

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/shirou/gopsutil/v4/process"
)

type Performance struct {
	kolideCount         int
	kolideRunningAsRoot bool
	data                map[string]any
}

func (p *Performance) Name() string {
	return "Process Report"
}

func (p *Performance) Run(ctx context.Context, _ io.Writer) error {

	p.data = make(map[string]any)
	ourPid := os.Getpid()

	_, err := process.NewProcessWithContext(ctx, int32(ourPid))
	if err != nil {
		return fmt.Errorf("getting process handle to self: %w", err)
	}

	// for _, proc := range ps {
	// 	// This doesn't feel great yet. I'm not sure what data we need, and some of the gopsutil stuff has
	// 	// weird extraneous exec routines. So this is a starting point.
	// 	pMap := map[string]any{
	// 		"pid":         proc.Pid,
	// 		"exe":         naIfError(proc.ExeWithContext(ctx)),
	// 		"cmdline":     naIfError(proc.CmdlineSliceWithContext(ctx)),
	// 		"create_time": naIfError(proc.CreateTimeWithContext(ctx)),
	// 		"ppid":        naIfError(proc.PpidWithContext(ctx)),
	// 		"mem_info":    naIfError(proc.MemoryInfoWithContext(ctx)),
	// 		"mem_info_ex": naIfError(proc.MemoryInfoExWithContext(ctx)),
	// 		"cpu_times":   naIfError(proc.TimesWithContext(ctx)),
	// 		"status":      naIfError(proc.StatusWithContext(ctx)),
	// 		"uids":        naIfError(proc.UidsWithContext(ctx)),
	// 		"username":    naIfError(proc.UsernameWithContext(ctx)),
	// 	}

	// 	exe, _ := proc.Exe()
	// 	if strings.Contains(strings.ToLower(exe), "kolide") {
	// 		p.kolideCount += 1

	// 		// Grab ENV vars -- available on windows/linux but not darwin;
	// 		// included primarily for troubleshooting browser opening issues
	// 		// on linux

	// 		p.data[fmt.Sprintf("%d", proc.Pid)] = pMap

	// 		if !p.kolideRunningAsRoot {
	// 			username := pMap["username"].(string)
	// 			if username == "root" || username == "NT AUTHORITY\\SYSTEM" {
	// 				p.kolideRunningAsRoot = true
	// 			}
	// 		}
	// 	}
	// }

	return nil
}

func (c *Performance) ExtraFileName() string {
	return ""
}

func (c *Performance) Status() Status {
	return Informational
}

func (c *Performance) Summary() string {
	return fmt.Sprintf("")
}

func (c *Performance) Data() any {
	return c.data
}
