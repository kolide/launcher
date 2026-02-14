package allowedcmd

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/kolide/launcher/ee/observability"
)

type TracedCmd struct {
	Ctx context.Context // nolint:containedctx // This is an approved usage of context for short lived cmd
	*exec.Cmd
}

// Start overrides the Start method to add tracing before executing the command.
func (t *TracedCmd) Start() error {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.Start() //nolint:forbidigo // This is our approved usage of t.Cmd.Start()
}

func (t *TracedCmd) String() string {
	return fmt.Sprintf("%+v", t.Args)
}

// Run overrides the Run method to add tracing before running the command.
func (t *TracedCmd) Run() error {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.Run() //nolint:forbidigo // This is our approved usage of t.Cmd.Run()
}

// Output overrides the Output method to add tracing before capturing output.
func (t *TracedCmd) Output() ([]byte, error) {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.Output() //nolint:forbidigo // This is our approved usage of t.Cmd.Output()
}

// CombinedOutput overrides the CombinedOutput method to add tracing before capturing combined output.
func (t *TracedCmd) CombinedOutput() ([]byte, error) {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.CombinedOutput() //nolint:forbidigo // This is our approved usage of t.Cmd.CombinedOutput()
}
