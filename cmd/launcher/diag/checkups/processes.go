package checkups

import (
	"fmt"
	"io"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type Processes struct {
}

func (c *Processes) Name() string {
	return "Process Report"
}

func (c *Processes) Run(short io.Writer) (string, error) {
	return checkupProcessReport(short)
}

// checkupProcessReport finds processes that look like Kolide launcher/osquery processes
func checkupProcessReport(short io.Writer) (string, error) {
	ps, err := process.Processes()
	if err != nil {
		return "", fmt.Errorf("No processes found")
	}

	var foundKolide bool
	for _, p := range ps {
		exe, _ := p.Exe()

		if strings.Contains(strings.ToLower(exe), "kolide") {
			foundKolide = true
			name, _ := p.Name()
			args, _ := p.Cmdline()
			user, _ := p.Username()
			info(short, fmt.Sprintf("%s\t%d\t%s\t%s", user, p.Pid, name, args))
		}
	}

	if !foundKolide {
		return "", fmt.Errorf("No launcher processes found")
	}
	return "Launcher processes found", nil
}
