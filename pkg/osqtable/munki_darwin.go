// +build darwin

package osqtable

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/groob/plist"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type MunkiInfo struct {
	report *munkiReport
}

func (m *MunkiInfo) MunkiReport(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("version"),
		table.TextColumn("start_time"),
		table.TextColumn("end_time"),
		table.TextColumn("success"),
		table.TextColumn("errors"),
		table.TextColumn("warnings"),
		table.TextColumn("console_user"),
		table.TextColumn("manifest_name"),
	}
	return table.NewPlugin("kolide_munki_report", columns, m.generateMunkiReport)
}

func (m *MunkiInfo) ManagedInstalls(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("installed_version"),
		table.TextColumn("installed"),
		table.TextColumn("name"),
		table.TextColumn("end_time"),
	}
	return table.NewPlugin("kolide_munki_installs", columns, m.generateMunkiInstalls)
}

func (m *MunkiInfo) generateMunkiInstalls(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	if err := m.loadReport(); err != nil {
		return nil, err
	}
	var results []map[string]string

	for _, install := range m.report.ManagedInstalls {
		results = append(results, map[string]string{
			"installed_version": install.InstalledVersion,
			"installed":         fmt.Sprintf("%v", install.Installed),
			"name":              install.Name,
			"end_time":          m.report.EndTime,
		})
	}

	return results, nil
}

func (m *MunkiInfo) generateMunkiReport(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	if err := m.loadReport(); err != nil {
		return nil, err
	}

	errors := strings.Join(m.report.Errors, ";")
	warnings := strings.Join(m.report.Warnings, ";")

	results := []map[string]string{
		map[string]string{
			"start_time":    m.report.StartTime,
			"end_time":      m.report.EndTime,
			"console_user":  m.report.ConsoleUser,
			"version":       m.report.ManagedInstallVersion,
			"success":       fmt.Sprintf("%v", len(m.report.Errors) == 0),
			"errors":        errors,
			"warnings":      warnings,
			"manifest_name": m.report.ManifestName,
		},
	}

	return results, nil
}

type munkiReport struct {
	ConsoleUser           string
	StartTime             string
	EndTime               string
	Errors                []string
	Warnings              []string
	ManagedInstallVersion string
	ManifestName          string
	ManagedInstalls       []managedInstall
}

type managedInstall struct {
	Installed        bool   `plist:"installed"`
	InstalledVersion string `plist:"installed_version"`
	Name             string `plist:"name"`
}

func (m *MunkiInfo) loadReport() error {
	const reportPath = "/Library/Managed Installs/ManagedInstallReport.plist"
	file, err := os.Open(reportPath)
	if err != nil {
		return errors.Wrap(err, "open ManagedInstallReport file")
	}
	defer file.Close()

	var report munkiReport
	if err := plist.NewDecoder(file).Decode(&report); err != nil {
		return errors.Wrap(err, "decode ManagedInstallReport plist")
	}
	m.report = &report
	return nil
}
