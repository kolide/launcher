package dataflattentable

import (
	"context"
	"os/exec"
	"testing"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/stretchr/testify/assert"
)

type mockCommand struct {
	name string
}

func (m mockCommand) Cmd(ctx context.Context, args ...string) (*allowedcmd.TracedCmd, error) {
	return &allowedcmd.TracedCmd{Cmd: exec.CommandContext(ctx, m.name, args...)}, nil
}

func (m mockCommand) Name() string { return m.name }

func TestDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tableName   string
		cmd         allowedcmd.AllowedCommand
		execArgs    []string
		description string
		expected    string
	}{
		{
			name:      "no description",
			tableName: "kolide_diskutil_list",
			cmd:       mockCommand{name: "diskutil"},
			execArgs:  []string{"list", "-plist"},
			expected:  "kolide_diskutil_list will exec the command `diskutil list -plist` and return the output.",
		},
		{
			name:        "with description",
			tableName:   "kolide_diskutil_list",
			cmd:         mockCommand{name: "diskutil"},
			execArgs:    []string{"list", "-plist"},
			description: "Returns information about disk partitions, volumes, and APFS containers on macOS. Useful for checking disk health or verifying APFS container structure.",
			expected:    "Returns information about disk partitions, volumes, and APFS containers on macOS. Useful for checking disk health or verifying APFS container structure.\n\nIt execs the command `diskutil list -plist`.",
		},
		{
			name:      "rundisclaimed command",
			tableName: "kolide_falconctl_stats",
			cmd:       mockCommand{name: "launcher"},
			execArgs:  []string{"rundisclaimed", "falconctl", "stats", "-p"},
			expected:  "kolide_falconctl_stats will exec the command `falconctl stats -p` and return the output.",
		},
		{
			name:        "rundisclaimed with description",
			tableName:   "kolide_falconctl_stats",
			cmd:         mockCommand{name: "launcher"},
			execArgs:    []string{"rundisclaimed", "falconctl", "stats", "-p"},
			description: "CrowdStrike Falcon sensor statistics including agent and cloud connectivity info.",
			expected:    "CrowdStrike Falcon sensor statistics including agent and cloud connectivity info.\n\nIt execs the command `falconctl stats -p`.",
		},
		{
			name:      "rundisclaimed with too few args",
			tableName: "kolide_broken",
			cmd:       mockCommand{name: "launcher"},
			execArgs:  []string{"rundisclaimed"},
			expected:  "kolide_broken will exec the command `~unknown~ rundisclaimed` and return the output.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tbl := &execTableV2{
				tableName:   tt.tableName,
				cmd:         tt.cmd,
				execArgs:    tt.execArgs,
				description: tt.description,
			}

			assert.Equal(t, tt.expected, tbl.Description())
		})
	}
}
