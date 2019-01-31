package wix

import (
	"encoding/xml"
	"io"
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
	InstallUninstallInstall InstallUninstallType = "install"
	InstallUninstallUnstall                      = "uninstall"
	InstallUninstallBoth                         = "both"
)

// ServiceInstall implements http://wixtoolset.org/documentation/manual/v3/xsd/wix/serviceinstall.html
type ServiceInstall struct {
	Account          string           `xml:",attr,omitempty"`
	Arguments        []string         `xml:",attr,omitempty"`
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

// Service is a rollup of ServiceInstall and ServiceControl
type Service struct {
	Binary string
}

func (s *Service) Xml(w io.Writer) error {
	si := ServiceInstall{
		Name:         "x",
		Id:           "ServiceInstall",
		Start:        StartAuto,
		Type:         "ownProcess",
		ErrorControl: ErrorControlNormal,
	}
	sc := ServiceControl{
		Name: "x",
		Id:   "ServiceControl",
	}

	enc := xml.NewEncoder(w)
	enc.Indent("                    ", "    ")
	if err := enc.Encode(si); err != nil {
		return err
	}
	if err := enc.Encode(sc); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	return nil

}
