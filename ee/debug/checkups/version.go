package checkups

import (
	"context"
	"fmt"
	"io"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/types"
)

type Version struct {
	k types.Knapsack
}

func (c *Version) Name() string {
	return "Launcher Version"
}

func (c *Version) Run(_ context.Context, fullFH io.Writer) error {
	return nil
}

func (c *Version) ExtraFileName() string {
	return ""
}

func (c *Version) Status() Status {
	return Informational
}

func (c *Version) Summary() string {
	return fmt.Sprintf("launcher_version %s", version.Version().Version)
}

func (c *Version) Data() any {
	return map[string]any{
		"update_channel":   c.k.UpdateChannel(),
		"tufServer":        c.k.TufServerURL(),
		"launcher_version": version.Version().Version,
	}
}
