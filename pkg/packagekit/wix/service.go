package wix

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
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
	StartDisabled           = "disabled:"
	StartBoot               = "boot"
	StartSystem             = "system"
)

type InstallUninstallType string

const (
	InstallUninstallInstall   InstallUninstallType = "install"
	InstallUninstallUninstall                      = "uninstall"
	InstallUninstallBoth                           = "both"
)

// ServiceInstall implements http://wixtoolset.org/documentation/manual/v3/xsd/wix/serviceinstall.html
type ServiceInstall struct {
	Account          string           `xml:",attr,omitempty"`
	Arguments        string           `xml:",attr,omitempty"`
	Description      string           `xml:",attr,omitempty"`
	DisplayName      string           `xml:",attr,omitempty"`
	EraseDescription bool             `xml:",attr,omitempty"`
	ErrorControl     ErrorControlType `xml:",attr,omitempty"`
	Id               string           `xml:",attr,omitempty"`
	Interactive      YesNoType        `xml:",attr,omitempty"`
	LoadOrderGroup   string           `xml:",attr,omitempty"`
	Name             string           `xml:",attr,omitempty"`
	Password         string           `xml:",attr,omitempty"`
	Start            StartType        `xml:",attr,omitempty"`
	Type             string           `xml:",attr,omitempty"`
	Vital            YesNoType        `xml:",attr,omitempty"`
}

// ServiceControl implements http://wixtoolset.org/documentation/manual/v3/xsd/wix/servicecontrol.html
type ServiceControl struct {
	Name   string               `xml:",attr,omitempty"`
	Id     string               `xml:",attr,omitempty"`
	Remove InstallUninstallType `xml:",attr,omitempty"`
	Start  InstallUninstallType `xml:",attr,omitempty"`
	Stop   InstallUninstallType `xml:",attr,omitempty"`
	Wait   YesNoType            `xml:",attr,omitempty"`
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
		s.serviceControl.Id = name
		s.serviceControl.Name = name
		s.serviceInstall.Id = name
		s.serviceInstall.Name = name
	}
}

func ServiceDescription(desc string) ServiceOpt {
	return func(s *Service) {
		s.serviceInstall.Description = desc
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
	defaultName := strings.TrimSuffix(matchString, ".exe") + "Svc"

	si := &ServiceInstall{
		Name:         defaultName,
		Id:           defaultName,
		Account:      `NT AUTHORITY\SYSTEM`,
		Start:        StartAuto,
		Type:         "ownProcess",
		ErrorControl: ErrorControlNormal,
		Vital:        Yes,
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

// Match returns a bool if there's a match
func (s *Service) Match(line string) (bool, error) {
	isMatch := strings.Contains(line, s.matchString)

	if isMatch {
		s.count += 1
	}

	if s.count > s.expectedCount {
		return isMatch, errors.Errorf("Too many matches. Have %d, expected %d", s.count, s.expectedCount)
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
