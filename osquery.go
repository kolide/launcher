package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// OsqueryInstallInfo holds info about the osquery installation
type OsqueryInstallInfo struct {
	// Whether osquery is installed
	IsInstalled bool
	// Osquery version as reported by osqueryi
	Version OsqueryVersion
	// Path to osqueryi executable
	OsqueryiPath string
	// Path to osqueryd executable
	OsquerydPath string
	// PID of osqueryd proccess if running
	OsquerydPID int
}

func RunQuery(query string, rows interface{}) error {
	// Run query in osqueryi
	cmd := exec.Command("osqueryi", "--json", query)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return err
	}

	// Parse JSON result
	return json.NewDecoder(&out).Decode(rows)
}

func DetectOsquery() (OsqueryInstallInfo, error) {
	info := OsqueryInstallInfo{IsInstalled: false}

	// Find osqueryi path
	info.OsqueryiPath, _ = exec.LookPath("osqueryi")
	if info.OsqueryiPath == "" {
		return info, nil
	}
	info.IsInstalled = true
	// Find osqueryd path
	info.OsquerydPath, _ = exec.LookPath("osqueryd")

	// Query for version
	var infoRows []struct {
		Version string `json:"version"`
	}
	err := RunQuery("SELECT version FROM osquery_info", &infoRows)
	if err != nil {
		return info, err
	}
	if len(infoRows) != 1 {
		return info, fmt.Errorf("expected 1 result for osquery_info, got: %d", len(infoRows))
	}
	info.Version = OsqueryVersion(infoRows[0].Version)

	// Query for osqueryd PID info
	var processRows []struct {
		PID string `json:"pid"`
	}
	err = RunQuery("SELECT pid FROM processes WHERE name LIKE 'osqueryd%' ORDER BY pid ASC", &processRows)
	if err != nil {
		return info, err
	}
	if len(processRows) > 0 {
		info.OsquerydPID, err = strconv.Atoi(processRows[0].PID)
		if err != nil {
			return info, err
		}
	}

	return info, nil
}
