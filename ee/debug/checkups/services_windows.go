//go:build windows
// +build windows

package checkups

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/launcher"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type servicesCheckup struct {
	k                         types.Knapsack
	data                      map[string]any
	serviceState              svc.State
	serviceStateHumanReadable string
	queryServiceStateErr      error
	queryServiceConfigErr     error
}

func (s *servicesCheckup) Name() string {
	return "Service Report"
}

func (s *servicesCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	serviceManager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}

	s.data = make(map[string]any)

	// Fetch information about Kolide service
	identifier := launcher.DefaultLauncherIdentifier
	if s.k.Identifier() != "" {
		identifier = s.k.Identifier()
	}
	kolideSvcName := launcher.ServiceName(identifier)
	serviceHandle, err := serviceManager.OpenService(kolideSvcName)
	if err != nil {
		return fmt.Errorf("opening service: %w", err)
	}
	defer serviceHandle.Close()

	s.data["service_name"] = kolideSvcName

	// Get status
	if status, err := serviceHandle.Query(); err != nil {
		s.queryServiceStateErr = err
		s.data["state"] = fmt.Sprintf("error: %v", err)
	} else {
		// Map state (uint32) to human-readable statuses
		s.serviceState = status.State
		s.serviceStateHumanReadable = stateHumanReadable(status.State)
		s.data["state_raw"] = status.State
		s.data["state"] = s.serviceStateHumanReadable
	}

	// Get config
	if cfg, err := serviceHandle.Config(); err != nil {
		s.queryServiceConfigErr = err
		s.data["config"] = fmt.Sprintf("error: %v", err)
	} else {
		s.data["binary_path_name"] = cfg.BinaryPathName
		s.data["load_order_group"] = cfg.LoadOrderGroup
		s.data["dependencies"] = cfg.Dependencies
		s.data["service_start_name"] = cfg.ServiceStartName
		s.data["display_name"] = cfg.DisplayName
		s.data["description"] = cfg.Description
		s.data["delayed_auto_start"] = cfg.DelayedAutoStart
		s.data["tag_id"] = cfg.TagId

		s.data["service_type_raw"] = cfg.ServiceType
		s.data["service_type"] = serviceTypeHumanReadable(cfg.ServiceType)

		s.data["start_type_raw"] = cfg.StartType
		s.data["start_type"] = startTypeHumanReadable(cfg.StartType)

		s.data["error_control_raw"] = cfg.ErrorControl
		s.data["error_control"] = errorControlHumanReadable(cfg.ErrorControl)

		s.data["sid_type_raw"] = cfg.SidType
		s.data["sid_type"] = sidTypeHumanReadable(cfg.SidType)
	}

	// If we're running doctor and not flare, no need to write extra data
	if extraWriter == io.Discard {
		return nil
	}

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	if err := gatherServices(extraZip, serviceManager); err != nil {
		return fmt.Errorf("gathering service list: %w", err)
	}

	if err := gatherServiceManagerEventLogs(ctx, extraZip, kolideSvcName); err != nil {
		return fmt.Errorf("gathering service manager event logs: %w", err)
	}

	if err := gatherServiceManagerEvents(ctx, extraZip, kolideSvcName); err != nil {
		return fmt.Errorf("gathering service manager events: %w", err)
	}

	return nil
}

func stateHumanReadable(state svc.State) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys/windows/svc#pkg-constants
	switch state {
	case windows.SERVICE_RUNNING:
		return "SERVICE_RUNNING"
	case windows.SERVICE_STOPPED:
		return "SERVICE_STOPPED"
	case windows.SERVICE_START_PENDING:
		return "SERVICE_START_PENDING"
	case windows.SERVICE_STOP_PENDING:
		return "SERVICE_STOP_PENDING"
	case windows.SERVICE_CONTINUE_PENDING:
		return "SERVICE_CONTINUE_PENDING"
	case windows.SERVICE_PAUSE_PENDING:
		return "SERVICE_PAUSE_PENDING"
	case windows.SERVICE_PAUSED:
		return "SERVICE_PAUSED"
	default:
		return fmt.Sprintf("unknown: %d", state)
	}
}

func serviceTypeHumanReadable(serviceType uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys@v0.11.0/windows#SERVICE_KERNEL_DRIVER
	switch serviceType {
	case windows.SERVICE_KERNEL_DRIVER:
		return "SERVICE_KERNEL_DRIVER"
	case windows.SERVICE_FILE_SYSTEM_DRIVER:
		return "SERVICE_FILE_SYSTEM_DRIVER"
	case windows.SERVICE_ADAPTER:
		return "SERVICE_ADAPTER"
	case windows.SERVICE_RECOGNIZER_DRIVER:
		return "SERVICE_RECOGNIZER_DRIVER"
	case windows.SERVICE_WIN32_OWN_PROCESS:
		return "SERVICE_WIN32_OWN_PROCESS"
	case windows.SERVICE_WIN32_SHARE_PROCESS:
		return "SERVICE_WIN32_SHARE_PROCESS"
	case windows.SERVICE_WIN32:
		return "SERVICE_WIN32"
	case windows.SERVICE_INTERACTIVE_PROCESS:
		return "SERVICE_INTERACTIVE_PROCESS"
	case windows.SERVICE_DRIVER:
		return "SERVICE_DRIVER"
	case windows.SERVICE_TYPE_ALL:
		return "SERVICE_TYPE_ALL"
	default:
		return fmt.Sprintf("unknown: %d", serviceType)
	}
}

func startTypeHumanReadable(startType uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys/windows/svc/mgr#pkg-constants
	// and https://pkg.go.dev/golang.org/x/sys@v0.11.0/windows#SERVICE_BOOT_START
	switch startType {
	case windows.SERVICE_BOOT_START:
		return "SERVICE_BOOT_START"
	case windows.SERVICE_SYSTEM_START:
		return "SERVICE_SYSTEM_START"
	case windows.SERVICE_AUTO_START:
		return "SERVICE_AUTO_START"
	case windows.SERVICE_DEMAND_START:
		return "SERVICE_DEMAND_START"
	case windows.SERVICE_DISABLED:
		return "SERVICE_DISABLED"
	default:
		return fmt.Sprintf("unknown: %d", startType)
	}
}

func errorControlHumanReadable(errorControl uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys/windows/svc/mgr#pkg-constants
	switch errorControl {
	case windows.SERVICE_ERROR_IGNORE:
		return "SERVICE_ERROR_IGNORE"
	case windows.SERVICE_ERROR_NORMAL:
		return "SERVICE_ERROR_NORMAL"
	case windows.SERVICE_ERROR_SEVERE:
		return "SERVICE_ERROR_SEVERE"
	case windows.SERVICE_ERROR_CRITICAL:
		return "SERVICE_ERROR_CRITICAL"
	default:
		return fmt.Sprintf("unknown: %d", errorControl)
	}
}

func sidTypeHumanReadable(sidType uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys@v0.11.0/windows#SERVICE_SID_TYPE_NONE
	// See also: https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_sid_info
	switch sidType {
	case windows.SERVICE_SID_TYPE_NONE:
		return "SERVICE_SID_TYPE_NONE"
	case windows.SERVICE_SID_TYPE_UNRESTRICTED:
		return "SERVICE_SID_TYPE_UNRESTRICTED"
	case windows.SERVICE_SID_TYPE_RESTRICTED:
		return "SERVICE_SID_TYPE_RESTRICTED"
	default:
		return fmt.Sprintf("unknown: %d", sidType)
	}
}

func gatherServices(z *zip.Writer, serviceManager *mgr.Mgr) error {
	services, err := serviceManager.ListServices()
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}

	servicesOut, err := z.Create("services.json")
	if err != nil {
		return fmt.Errorf("creating services.json: %w", err)
	}

	servicesRaw, err := json.Marshal(services)
	if err != nil {
		return fmt.Errorf("marshalling services: %w", err)
	}

	if _, err := servicesOut.Write(servicesRaw); err != nil {
		return fmt.Errorf("writing services: %w", err)
	}

	return nil
}

// gatherServiceManagerEvents uses Get-WinEvent to fetch the service manager logs. This might be newer than Get-EventLog
func gatherServiceManagerEvents(ctx context.Context, z *zip.Writer, kolideSvcName string) error {
	out, err := z.Create("eventlog-Get-WinEvent.json")
	if err != nil {
		return fmt.Errorf("creating eventlog-Get-WinEvent.json: %w", err)
	}

	filterExpression := fmt.Sprintf(`@{LogName='System'; ProviderName='Service Control Manager'; Data='%s'}`, kolideSvcName)

	cmdArgs := []string{
		"Get-WinEvent",
		"-MaxEvents 100",
		"-FilterHashtable", filterExpression,
		"|",
		"ConvertTo-Json",
	}

	cmd, err := allowedcmd.Powershell(ctx, cmdArgs...)
	if err != nil {
		return fmt.Errorf("creating powershell command: %w", err)
	}
	hideWindow(cmd.Cmd)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running Get-WinEvent: error %w", err)
	}

	return nil
}

// gatherServiceManagerEventLogs uses Get-EventLog to getch service manager logs. This might be a legacy path
func gatherServiceManagerEventLogs(ctx context.Context, z *zip.Writer, kolideSvcName string) error {
	eventLogOut, err := z.Create("eventlog-Get-EventLog.txt")
	if err != nil {
		return fmt.Errorf("creating eventlog-Get-EventLog.txt: %w", err)
	}

	cmdletArgs := []string{
		"Get-EventLog",
		"-Newest", "100",
		"-LogName", "System",
		"-Source", "\"Service Control Manager\"",
		"-Message", fmt.Sprintf("*%s*", kolideSvcName),
		"|",
		"Format-Table", "-Wrap", "-AutoSize", // ensure output doesn't get truncated
	}

	getEventLogCmd, err := allowedcmd.Powershell(ctx, cmdletArgs...)
	if err != nil {
		return fmt.Errorf("creating powershell command: %w", err)
	}
	hideWindow(getEventLogCmd.Cmd)
	getEventLogCmd.Stdout = eventLogOut
	getEventLogCmd.Stderr = eventLogOut
	if err := getEventLogCmd.Run(); err != nil {
		return fmt.Errorf("running Get-EventLog: error %w", err)
	}

	return nil
}

func (s *servicesCheckup) ExtraFileName() string {
	return "servicemgr.zip"
}

func (s *servicesCheckup) Status() Status {
	if s.serviceState == svc.Running {
		return Passing
	}

	return Failing
}

func (s *servicesCheckup) Summary() string {
	if len(s.data) == 0 {
		return "did not find Kolide service"
	}

	if s.queryServiceStateErr != nil && s.queryServiceConfigErr != nil {
		return fmt.Sprintf("found Kolide service but could not query state (%s) or config (%s)", s.queryServiceStateErr.Error(), s.queryServiceConfigErr.Error())
	}

	if s.queryServiceStateErr != nil {
		return fmt.Sprintf("found Kolide service but could not query state: %s", s.queryServiceStateErr.Error())
	}

	if s.queryServiceConfigErr != nil {
		return fmt.Sprintf("found Kolide service in state %d (%s) but could not query config: %s", s.serviceState, s.serviceStateHumanReadable, s.queryServiceConfigErr.Error())
	}

	return fmt.Sprintf("found Kolide service in state %d (%s)", s.serviceState, s.serviceStateHumanReadable)
}

func (s *servicesCheckup) Data() any {
	return s.data
}
