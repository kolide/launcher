//go:build linux
// +build linux

package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/allowedcmd"
)

type coredumpCheckup struct {
	status  Status
	summary string
	data    map[string]any
}

func (c *coredumpCheckup) Name() string {
	return "Coredump Report"
}

func (c *coredumpCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	c.data = make(map[string]any)

	c.status = Passing
	for _, binaryName := range []string{"launcher", "osqueryd"} {
		coredumpListRaw, err := c.coredumpList(ctx, binaryName)
		if err != nil {
			c.summary += fmt.Sprintf("could not get coredump data for %s; ", binaryName)
			c.data[binaryName] = fmt.Sprintf("listing coredumps: %v", err)
			continue
		}

		if coredumpListRaw == nil {
			c.summary += fmt.Sprintf("%s does not have any coredumps; ", binaryName)
			c.data[binaryName] = "N/A"
			continue
		}

		// At least one coredump exists for at least one binary
		c.status = Warning
		c.summary += fmt.Sprintf("%s has at least one coredump; ", binaryName)
		c.data[binaryName] = strings.TrimSpace(string(coredumpListRaw))
	}
	c.summary = strings.TrimSuffix(strings.TrimSpace(c.summary), ";")

	if extraWriter == io.Discard || c.status == Passing {
		// Either not a flare, or we don't have any coredumps to grab info about
		return nil
	}

	// Gather extra information about the coredumps
	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()
	for _, binaryName := range []string{"launcher", "osqueryd"} {
		if c.data[binaryName] == "N/A" {
			continue
		}
		if err := c.writeCoredumpInfo(ctx, binaryName, extraZip); err != nil {
			fmt.Fprintf(extraWriter, "Writing coredump info for %s: %v", binaryName, err)
		}
	}

	return nil
}

func (c *coredumpCheckup) coredumpList(ctx context.Context, binaryName string) ([]byte, error) {
	coredumpctlListCmd, err := allowedcmd.Coredumpctl(ctx, "--no-pager", "--no-legend", "--json=short", "list", binaryName)
	if err != nil {
		return nil, fmt.Errorf("could not create coredumpctl command: %w", err)
	}

	out, err := coredumpctlListCmd.CombinedOutput()
	if strings.Contains(string(out), "No coredumps found") {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("running coredumpctl list %s: out `%s`; %w", binaryName, string(out), err)
	}

	return out, nil
}

func (c *coredumpCheckup) writeCoredumpInfo(ctx context.Context, binaryName string, z *zip.Writer) error {
	// Print info about all matching coredumps
	coredumpctlInfoCmd, err := allowedcmd.Coredumpctl(ctx, "--no-pager", "info", binaryName)
	if err != nil {
		return fmt.Errorf("could not create coredumpctl info command: %w", err)
	}
	infoOut, err := coredumpctlInfoCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running coredumpctl info %s: out `%s`; %w", binaryName, string(infoOut), err)
	}
	coredumpctlInfoFile, err := z.Create(filepath.Join(".", fmt.Sprintf("coredumpctl-info-%s.txt", binaryName)))
	if err != nil {
		return fmt.Errorf("creating coredumpctl-info.txt for %s in zip: %w", binaryName, err)
	}
	if _, err := coredumpctlInfoFile.Write(infoOut); err != nil {
		return fmt.Errorf("writing coredumpctl-info.txt in %s zip: %w", binaryName, err)
	}

	// Now, try to get a coredump -- this will grab the most recent one
	tempDir, err := agent.MkdirTemp("coredump-flare")
	if err != nil {
		return fmt.Errorf("making temporary directory for coredump from %s: %w", binaryName, err)
	}
	defer os.RemoveAll(tempDir)
	tempDumpFile := filepath.Join(tempDir, fmt.Sprintf("coredump-%s.dump", binaryName))
	coredumpctlDumpCmd, err := allowedcmd.Coredumpctl(ctx, "--no-pager", fmt.Sprintf("--output=%s", tempDumpFile), "dump", binaryName)
	if err != nil {
		return fmt.Errorf("could not create coredumpctl dump command: %w", err)
	}
	if err := coredumpctlDumpCmd.Run(); err != nil {
		return fmt.Errorf("running coredumpctl dump %s: %w", binaryName, err)
	}
	if err := addFileToZip(z, tempDumpFile); err != nil {
		return fmt.Errorf("adding coredumpctl dump %s output file to zip: %w", binaryName, err)
	}

	return nil
}

func (c *coredumpCheckup) ExtraFileName() string {
	return "coredumps.zip"
}

func (c *coredumpCheckup) Status() Status {
	return c.status
}

func (c *coredumpCheckup) Summary() string {
	return c.summary
}

func (c *coredumpCheckup) Data() any {
	return c.data
}
