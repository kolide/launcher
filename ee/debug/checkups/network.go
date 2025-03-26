package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
)

type networkCheckup struct {
	status  Status
	summary string
}

func (n *networkCheckup) Name() string {
	return "Network Report"
}

func (n *networkCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Confirm that we can listen on the local network -- launcher has to be able to do this
	// in order to communicate with desktop processes
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		n.status = Failing
		n.summary = fmt.Sprintf("launcher cannot listen on local network: %v", err)
	} else {
		listener.Close()
		n.status = Passing
		n.summary = "launcher can listen on local network"
	}

	if extraWriter == io.Discard {
		return nil
	}

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	// This will put all the command stdout into a single commands.md. It's not yet clear if we want to split them up
	// or combine them
	commandOutput, err := extraZip.Create("commands.md")
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}

	for _, c := range listCommands() {
		runCommand(ctx, c, commandOutput)
	}

	for _, fileLocation := range listFiles() {
		_ = addFileToZip(extraZip, fileLocation)
	}

	return nil
}

func runCommand(ctx context.Context, c networkCommand, commandOutput io.Writer) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd, err := c.cmd(ctx, c.args...)
	if err != nil {
		return
	}
	_ = runCmdMarkdownLogged(cmd.Cmd, commandOutput)
}

func (n *networkCheckup) ExtraFileName() string {
	return "network.zip"
}

func (n *networkCheckup) Status() Status {
	return n.status
}

func (n *networkCheckup) Summary() string {
	return n.summary
}

func (n *networkCheckup) Data() any {
	return nil
}

type networkCommand struct {
	cmd  allowedcmd.AllowedCommand
	args []string
}
