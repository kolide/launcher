package checkups

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/allowedpaths"
)

type gnomeExtensions struct {
	status  Status
	summary string
}

var expectedExtensions = []string{
	"ubuntu-appindicators@ubuntu.com",
}

const (
	runDir = "/run/user"
)

func (c *gnomeExtensions) Name() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	return "Gnome Extensions"
}

func (c *gnomeExtensions) ExtraFileName() string {
	return "extensions.log"
}

func (c *gnomeExtensions) Run(ctx context.Context, extraWriter io.Writer) error {
	fmt.Fprintf(extraWriter, "# Checking Gnome Extensions\n\n")

	rundirs, err := os.ReadDir(runDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", runDir, err)
	}

	for _, dir := range rundirs {
		if !dir.IsDir() {
			continue
		}

		status, summary := checkRundir(ctx, extraWriter, filepath.Join(runDir, dir.Name()))

		if status != Passing {
			c.status = status
		}

		if c.summary == "" {
			c.summary = fmt.Sprintf("uid:%s: %s", dir.Name(), summary)
		} else {
			c.summary = fmt.Sprintf("%s; uid:%s: %s", c.summary, dir.Name(), summary)
		}

	}

	// If we got here, without setting c.status, it must be passing. It feels not great assuming that,
	// but it's a low risk place, and the code is cleaner.
	if c.status == "" || c.status == Unknown {
		c.status = Passing
	}

	return nil
}

func (c *gnomeExtensions) Status() Status {
	return c.status
}

func (c *gnomeExtensions) Summary() string {
	return c.summary
}

func (c *gnomeExtensions) Data() any {
	return nil
}

func execGnomeExtension(ctx context.Context, extraWriter io.Writer, rundir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// TODO: Need to figure out how to make this run per user
	// pkg/osquery/tables/gsettings/gsettings.go probably has appropriate prior art.
	// But do we really want the forloop?

	cmd, err := allowedpaths.CommandContextWithLookup(ctx, "gnome-extensions", args...)
	if err != nil {
		return nil, fmt.Errorf("creating gnome-extensions command: %w", err)
	}

	// gnome seems to do things through this env
	cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_RUNTIME_DIR=%s", rundir))

	buf := &bytes.Buffer{}
	cmd.Stderr = io.MultiWriter(extraWriter, buf)
	cmd.Stdout = cmd.Stderr

	// A bit of an experiment in output formatting. Make it look like a markdown command block
	fmt.Fprintf(extraWriter, "```\n$ %s\n", cmd.String())
	defer fmt.Fprintf(extraWriter, "```\n\n")

	if err := cmd.Run(); err != nil {
		// reset the buffer so we don't return the error code
		return nil, fmt.Errorf(`running "%s", err is: %s: %w`, cmd.String(), buf.String(), err)
	}

	return buf.Bytes(), nil
}

func checkRundir(ctx context.Context, extraWriter io.Writer, rundir string) (Status, string) {
	fmt.Fprintf(extraWriter, "## Checking rundir %s\n\n", rundir)

	status := Unknown
	summary := "unknown"

	missing := []string{}

	for _, ext := range expectedExtensions {
		fmt.Fprintf(extraWriter, "### %s\n\n", ext)

		output, err := execGnomeExtension(ctx, extraWriter, rundir, "show", ext)
		if err != nil {
			// Errors running this command are probably fatal, may as well bail
			return Erroring, fmt.Sprintf("error running gnome-extensions: %s", err)
		}

		// Is it enabled?
		if !bytes.Contains(output, []byte("State: ENABLED")) {
			missing = append(missing, ext)
		}
	}

	if len(missing) > 0 {
		status = Failing
		summary = fmt.Sprintf("missing (or screenlocked) extensions: %s", strings.Join(missing, ", "))
	} else {
		status = Passing
		summary = fmt.Sprintf("enabled extensions: %s", strings.Join(expectedExtensions, ", "))
	}

	if extraWriter != io.Discard {
		// We can ignore the response, because it's tee'ed into extraWriter
		_, _ = execGnomeExtension(ctx, extraWriter, rundir, "list")

	}

	return status, summary

}
