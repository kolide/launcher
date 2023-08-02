package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
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

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	// Gather results of ipconfig/ifconfig for extra writer
	if err := gatherNetworkConfiguration(ctx, extraZip); err != nil {
		return fmt.Errorf("gathering networkconfig: %w", err)
	}

	// Gather /etc/hosts for posix systems for extra writer
	if err := gatherEtcHosts(extraZip); err != nil {
		return fmt.Errorf("gathering /etc/hosts: %w", err)
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

// gatherNetworkConfiguration runs ifconfig on linux/macos and ipconfig on windows,
// writing the ouptut to the given zip writer
func gatherNetworkConfiguration(ctx context.Context, z *zip.Writer) error {
	var configCommand string
	var configArgs []string
	switch runtime.GOOS {
	case "darwin", "linux":
		configCommand = "ifconfig"
		configArgs = []string{"-a"}
	case "windows":
		configCommand = "ipconfig"
		configArgs = []string{"/all"}
	default:
		return fmt.Errorf("not supported for %s", runtime.GOOS)
	}

	if _, err := exec.LookPath(configCommand); err != nil {
		// Not installed, nothing we can do here
		return nil
	}

	cmd := exec.CommandContext(ctx, configCommand, configArgs...)
	cmdOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running command %s %v: %w", configCommand, configArgs, err)
	}

	out, err := z.Create("networkconfig")
	if err != nil {
		return fmt.Errorf("creating networkconfig: %w", err)
	}

	if _, err := out.Write(cmdOutput); err != nil {
		return fmt.Errorf("writing network config: %w", err)
	}

	return nil
}

// gatherEtcHosts returns the contents of the /etc/hosts file on posix systems
func gatherEtcHosts(z *zip.Writer) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	out, err := z.Create("etchosts")
	if err != nil {
		return fmt.Errorf("creating etc hosts: %w", err)
	}

	etcHostsFile, err := os.Open("/etc/hosts")
	if err != nil {
		return fmt.Errorf("opening /etc/hosts: %w", err)
	}
	defer etcHostsFile.Close()

	etcHostsRaw, err := io.ReadAll(etcHostsFile)
	if err != nil {
		return fmt.Errorf("reading /etc/hosts: %w", err)
	}

	if _, err := out.Write(etcHostsRaw); err != nil {
		return fmt.Errorf("writing /etc/hosts contents to zip: %w", err)
	}

	return nil
}
