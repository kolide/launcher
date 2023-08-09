//go:build windows
// +build windows

package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const kolideSvcName = "LauncherKolideK2Svc"

type servicesCheckup struct {
	data                      map[string]any
	serviceState              svc.State
	serviceStateHumanReadable string
	queryServiceStateErr      error
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
	if extraWriter == nil {
		return nil
	}

	// Write names of all other services to extra writer too
	services, err := serviceManager.ListServices()
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}
	_ = json.NewEncoder(extraWriter).Encode(services)

	return nil
}

func stateHumanReadable(state svc.State) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys/windows/svc#pkg-constants
	switch state {
	case svc.Running:
		return "SERVICE_RUNNING"
	case svc.Stopped:
		return "SERVICE_STOPPED"
	case svc.StartPending:
		return "SERVICE_START_PENDING"
	case svc.StopPending:
		return "SERVICE_STOP_PENDING"
	case svc.ContinuePending:
		return "SERVICE_CONTINUE_PENDING"
	case svc.PausePending:
		return "SERVICE_PAUSE_PENDING"
	case svc.Paused:
		return "SERVICE_PAUSED"
	default:
		return fmt.Sprintf("unknown: %d", state)
	}
}

func serviceTypeHumanReadable(serviceType uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys@v0.11.0/windows#SERVICE_KERNEL_DRIVER
	switch serviceType {
	case 1:
		return "SERVICE_KERNEL_DRIVER"
	case 2:
		return "SERVICE_FILE_SYSTEM_DRIVER"
	case 4:
		return "SERVICE_ADAPTER"
	case 8:
		return "SERVICE_RECOGNIZER_DRIVER"
	case 16:
		return "SERVICE_WIN32_OWN_PROCESS"
	case 32:
		return "SERVICE_WIN32_SHARE_PROCESS"
	case 16 | 32:
		return "SERVICE_WIN32"
	case 256:
		return "SERVICE_INTERACTIVE_PROCESS"
	case 1 | 2 | 8:
		return "SERVICE_DRIVER"
	case 1 | 2 | 4 | 8 | 16 | 32 | 256:
		return "SERVICE_TYPE_ALL"
	default:
		return fmt.Sprintf("unknown: %d", serviceType)
	}
}

func startTypeHumanReadable(startType uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys/windows/svc/mgr#pkg-constants
	// and https://pkg.go.dev/golang.org/x/sys@v0.11.0/windows#SERVICE_BOOT_START
	switch startType {
	case 0:
		return "SERVICE_BOOT_START"
	case 1:
		return "SERVICE_SYSTEM_START"
	case mgr.StartAutomatic:
		return "SERVICE_AUTO_START"
	case mgr.StartManual:
		return "SERVICE_DEMAND_START"
	case mgr.StartDisabled:
		return "SERVICE_DISABLED"
	default:
		return fmt.Sprintf("unknown: %d", startType)
	}
}

func errorControlHumanReadable(errorControl uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys/windows/svc/mgr#pkg-constants
	switch errorControl {
	case mgr.ErrorIgnore:
		return "SERVICE_ERROR_IGNORE"
	case mgr.ErrorNormal:
		return "SERVICE_ERROR_NORMAL"
	case mgr.ErrorSevere:
		return "SERVICE_ERROR_SEVERE"
	case mgr.ErrorCritical:
		return "SERVICE_ERROR_CRITICAL"
	default:
		return fmt.Sprintf("unknown: %d", errorControl)
	}
}

func sidTypeHumanReadable(sidType uint32) string {
	// For mapping, see https://pkg.go.dev/golang.org/x/sys@v0.11.0/windows#SERVICE_SID_TYPE_NONE
	// See also: https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_sid_info
	switch sidType {
	case 0:
		return "SERVICE_SID_TYPE_NONE"
	case 1:
		return "SERVICE_SID_TYPE_UNRESTRICTED"
	case 3:
		return "SERVICE_SID_TYPE_RESTRICTED"
	default:
		return fmt.Sprintf("unknown: %d", sidType)
	}
}

func (s *servicesCheckup) ExtraFileName() string {
	return "services.json"
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

	if s.queryServiceStateErr != nil {
		return fmt.Sprintf("found Kolide service but could not query state: %s", s.queryServiceStateErr.Error())
	}

	return fmt.Sprintf("found Kolide service in state %d (%s)", s.serviceState, s.serviceStateHumanReadable)
}

func (s *servicesCheckup) Data() any {
	return s.data
}
