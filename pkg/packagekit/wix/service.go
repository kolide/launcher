package wix

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/serenize/snaker"
)

// http://wixtoolset.org/documentation/manual/v3/xsd/wix/serviceinstall.html
// http://wixtoolset.org/documentation/manual/v3/xsd/wix/servicecontrol.html
// https://helgeklein.com/blog/2014/09/real-world-example-wix-msi-application-installer/
type YesNoType string

const (
	Yes YesNoType = "yes"
	No            = "no"
)

type ErrorControlType string

const (
	ErrorControlIgnore   ErrorControlType = "ignore"
	ErrorControlNormal                    = "normal"
	ErrorControlCritical                  = "critical"
)

type StartType string

const (
	StartAuto     StartType = "auto"
	StartDemand             = "demand"
	StartDisabled           = "disabled"
	StartBoot               = "boot"
	StartSystem             = "system"
	StartNone               = ""
)

type InstallUninstallType string

const (
	InstallUninstallInstall   InstallUninstallType = "install"
	InstallUninstallUninstall                      = "uninstall"
	InstallUninstallBoth                           = "both"
)

// ServiceInstall implements http://wixtoolset.org/documentation/manual/v3/xsd/wix/serviceinstall.html
type ServiceInstall struct {
	Account           string             `xml:",attr,omitempty"`
	Arguments         string             `xml:",attr,omitempty"`
	Description       string             `xml:",attr,omitempty"`
	DisplayName       string             `xml:",attr,omitempty"`
	EraseDescription  bool               `xml:",attr,omitempty"`
	ErrorControl      ErrorControlType   `xml:",attr,omitempty"`
	Id                string             `xml:",attr,omitempty"`
	Interactive       YesNoType          `xml:",attr,omitempty"`
	LoadOrderGroup    string             `xml:",attr,omitempty"`
	Name              string             `xml:",attr,omitempty"`
	Password          string             `xml:",attr,omitempty"`
	Start             StartType          `xml:",attr,omitempty"`
	Type              string             `xml:",attr,omitempty"`
	Vital             YesNoType          `xml:",attr,omitempty"` // The overall install should fail if this service fails to install
	UtilServiceConfig *UtilServiceConfig `xml:",omitempty"`
	ServiceConfig     *ServiceConfig     `xml:",omitempty"`
}

// ServiceControl implements
// http://wixtoolset.org/documentation/manual/v3/xsd/wix/servicecontrol.html
type ServiceControl struct {
	Name   string               `xml:",attr,omitempty"`
	Id     string               `xml:",attr,omitempty"`
	Remove InstallUninstallType `xml:",attr,omitempty"`
	Start  InstallUninstallType `xml:",attr,omitempty"`
	Stop   InstallUninstallType `xml:",attr,omitempty"`
	Wait   YesNoType            `xml:",attr,omitempty"`
}

// ServiceConfig implements
// https://wixtoolset.org/documentation/manual/v3/xsd/wix/serviceconfig.html
// This is used needed to set DelayedAutoStart
type ServiceConfig struct {
	// TODO: this should need a namespace, and yet. See https://github.com/golang/go/issues/36813
	XMLName          xml.Name  `xml:"http://schemas.microsoft.com/wix/2006/wi ServiceConfig"`
	DelayedAutoStart YesNoType `xml:",attr,omitempty"`
	OnInstall        YesNoType `xml:",attr,omitempty"`
	OnReinstall      YesNoType `xml:",attr,omitempty"`
	OnUninstall      YesNoType `xml:",attr,omitempty"`
}

// UtilServiceConfig implements
// http://wixtoolset.org/documentation/manual/v3/xsd/util/serviceconfig.html
// This is used to set FailureActions. There are some
// limitations. Notably, reset period is in days here, though the
// underlying `sc.exe` command supports seconds. (See
// https://github.com/wixtoolset/issues/issues/5963)
//
// Docs are a bit confusing. This schema is supported, and should
// work. The non-util ServiceConfig generates unsupported CNDL1150
// errors.
type UtilServiceConfig struct {
	XMLName                      xml.Name `xml:"http://schemas.microsoft.com/wix/UtilExtension ServiceConfig"`
	FirstFailureActionType       string   `xml:",attr,omitempty"`
	SecondFailureActionType      string   `xml:",attr,omitempty"`
	ThirdFailureActionType       string   `xml:",attr,omitempty"`
	RestartServiceDelayInSeconds int      `xml:",attr,omitempty"`
	ResetPeriodInDays            int      `xml:",attr,omitempty"`
}

// Service represents a wix service. It provides an interface to both
// ServiceInstall and ServiceControl.
type Service struct {
	matchString   string
	count         int // number of times we've seen this. Used for error handling
	expectedCount int

	serviceInstall *ServiceInstall
	serviceControl *ServiceControl
}

type ServiceOpt func(*Service)

func ServiceName(name string) ServiceOpt {
	return func(s *Service) {
		s.serviceControl.Id = cleanServiceName(name)
		s.serviceControl.Name = cleanServiceName(name)
		s.serviceInstall.Id = cleanServiceName(name)
		s.serviceInstall.Name = cleanServiceName(name)
	}
}

func ServiceDescription(desc string) ServiceOpt {
	return func(s *Service) {
		s.serviceInstall.Description = desc
	}
}

func WithDelayedStart() ServiceOpt {
	return func(s *Service) {
		s.serviceInstall.ServiceConfig.DelayedAutoStart = Yes
	}
}

func WithDisabledService() ServiceOpt {
	return func(s *Service) {
		s.serviceInstall.Start = StartDisabled
		// If this is not explicitly set to none, the installer hangs trying to start the
		// disabled service.
		s.serviceControl.Start = StartNone
	}
}

// ServiceArgs takes an array of args, wraps them in spaces, then
// joins them into a string. Handling spaces in the arguments is a bit
// gnarly. Some parts of windows use ` as an escape character, but
// that doesn't seem to work here. However, we can use double quotes
// to wrap the whole string. But, in xml quotes are _always_
// transmitted as html entities -- &#34; or &quot;. Luckily, wix seems
// fine with that. It converts them back to double quotes when it
// makes the service
func ServiceArgs(args []string) ServiceOpt {
	return func(s *Service) {
		quotedArgs := make([]string, len(args))

		for i, arg := range args {
			if strings.ContainsAny(arg, " ") {
				quotedArgs[i] = fmt.Sprintf(`"%s"`, arg)
			} else {
				quotedArgs[i] = arg
			}
		}

		s.serviceInstall.Arguments = strings.Join(quotedArgs, " ")
	}
}

// New returns a service
func NewService(matchString string, opts ...ServiceOpt) *Service {
	// Set some defaults. It's not clear we can reset in under a
	// day. See https://github.com/wixtoolset/issues/issues/5963
	utilServiceConfig := &UtilServiceConfig{
		FirstFailureActionType:       "restart",
		SecondFailureActionType:      "restart",
		ThirdFailureActionType:       "restart",
		ResetPeriodInDays:            1,
		RestartServiceDelayInSeconds: 5,
	}

	serviceConfig := &ServiceConfig{
		OnInstall:   Yes,
		OnReinstall: Yes,
	}

	// If a service name is not specified, replace the .exe with a svc,
	// and CamelCase it. (eg: daemon.exe becomes DaemonSvc). It is
	// probably better to specific a ServiceName, but this might be an
	// okay default.
	defaultName := cleanServiceName(strings.TrimSuffix(matchString, ".exe") + ".svc")
	si := &ServiceInstall{
		Name:              defaultName,
		Id:                defaultName,
		Account:           `[SERVICEACCOUNT]`, // Wix resolves this to `LocalSystem`
		Start:             StartAuto,
		Type:              "ownProcess",
		ErrorControl:      ErrorControlNormal,
		Vital:             Yes,
		UtilServiceConfig: utilServiceConfig,
		ServiceConfig:     serviceConfig,
	}

	sc := &ServiceControl{
		Name:   defaultName,
		Id:     defaultName,
		Stop:   InstallUninstallBoth,
		Start:  InstallUninstallInstall,
		Remove: InstallUninstallUninstall,
		Wait:   No,
	}

	s := &Service{
		matchString:    matchString,
		expectedCount:  1,
		count:          0,
		serviceInstall: si,
		serviceControl: sc,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Match returns a bool if there's a match, and throws an error if we
// have too many matches. This is to ensure the configured regex isn't
// broader than expected.
func (s *Service) Match(line string) (bool, error) {
	isMatch := strings.Contains(line, s.matchString)

	if isMatch {
		s.count += 1
	}

	if s.count > s.expectedCount {
		return isMatch, fmt.Errorf("Too many matches. Have %d, expected %d. (on %s)", s.count, s.expectedCount, s.matchString)
	}

	return isMatch, nil
}

// Xml converts a Service resource to Xml suitable for embedding
func (s *Service) Xml(w io.Writer) error {

	enc := xml.NewEncoder(w)
	enc.Indent("                    ", "    ")
	if err := enc.Encode(s.serviceInstall); err != nil {
		return err
	}
	if err := enc.Encode(s.serviceControl); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	return nil

}

// cleanServiceName removes characters windows doesn't like in
// services names, and converts everything to camel case. Right now,
// it only removes likely bad characters. It is not as complete as an
// allowlist.
func cleanServiceName(in string) string {
	r := strings.NewReplacer(
		"-", "_",
		" ", "_",
		".", "_",
		"/", "_",
		"\\", "_",
	)

	return snaker.SnakeToCamel(r.Replace(in))
}
