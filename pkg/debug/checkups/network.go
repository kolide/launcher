package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"time"
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

	for _, commandArr := range listCommands() {
		if len(commandArr) < 1 {
			// how did this happen
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, commandArr[0], commandArr[1:]...)
		_ = runCmdMarkdownLogged(cmd, commandOutput)
	}

	for _, fileLocation := range listFiles() {
		_ = addFileToZip(extraZip, fileLocation)
	}

	return nil
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

func listCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{
			{"ifconfig", "-a"},
			{"netstat", "-nr"},
		}
	case "linux":
		return [][]string{
			{"ifconfig", "-a"},
			{"ip", "-N", "-d", "-h", "-a", "address"},
			{"ip", "-N", "-d", "-h", "-a", "route"},
		}
	case "windows":
		return [][]string{
			{"ipconfig", "/all"},
		}
	default:
		return nil
	}
}

func listFiles() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/etc/hosts",
			"/etc/resolv.conf",
		}
	case "linux":
		return []string{
			"/etc/nsswitch.conf",
			"/etc/hosts",
			"/etc/resolv.conf",
		}
	case "windows":
		return []string{}
	default:
		return nil
	}
}
