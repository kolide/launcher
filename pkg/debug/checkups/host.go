package checkups

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/types"
)

const osSqlQuery = `
SELECT
	os_version.build as os_build,
	os_version.name as os_name,
	os_version.version as os_version
FROM
	os_version;
`

const systemSqlQuery = `
SELECT
	system_info.hardware_model,
	system_info.hardware_serial,
	system_info.hardware_vendor,
	system_info.uuid as hardware_uuid
FROM
	system_info;
`

type (
	osqResp         []map[string]string
	hostInfoCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (hc *hostInfoCheckup) Data() any             { return hc.data }
func (hc *hostInfoCheckup) ExtraFileName() string { return "" }
func (hc *hostInfoCheckup) Name() string          { return "Host Info" }
func (hc *hostInfoCheckup) Status() Status        { return hc.status }
func (hc *hostInfoCheckup) Summary() string       { return hc.summary }

func (hc *hostInfoCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	hc.data = make(map[string]any)
	hc.data["hostname"] = hostName()
	hc.data["keyinfo"] = agentKeyInfo()
	hc.data["bbolt_db_size"] = hc.bboltDbSize()
	hc.data["user_desktop_processes"] = runner.InstanceDesktopProcessRecords()

	if runtime.GOOS == "windows" {
		hc.data["in_modern_standby"] = hc.k.InModernStandby()
	}

	hc.status = Informational
	hc.summary = fmt.Sprintf("\n\t\thostname: %s", hc.data["hostname"])

	if result, err := hc.osqueryInteractive(ctx, osSqlQuery); err != nil {
		hc.data["os_version"] = err.Error()
	} else {
		hc.data["os_version"] = result
		hc.appendResultsToSummary(result)
	}

	if result, err := hc.osqueryInteractive(ctx, systemSqlQuery); err != nil {
		hc.data["system_info"] = err.Error()
	} else {
		hc.data["system_info"] = result
		hc.appendResultsToSummary(result)
	}

	return nil
}

func (hc *hostInfoCheckup) appendResultsToSummary(results map[string]string) {
	for k, v := range results {
		hc.summary = fmt.Sprintf("%s\n\t\t%s: %s", hc.summary, k, v)
	}
}

func (hc *hostInfoCheckup) osqueryInteractive(ctx context.Context, query string) (map[string]string, error) {
	var launcherPath string
	switch runtime.GOOS {
	case "linux", "darwin":
		launcherPath = "/usr/local/kolide-k2/bin/launcher"
	case "windows":
		launcherPath = `C:\Program Files\Kolide\Launcher-kolide-k2\bin\launcher.exe`
	}

	cmdCtx, cmdCancel := context.WithTimeout(ctx, 20*time.Second)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, launcherPath, "interactive", "--osquery_flag=json")
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader(query)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %s interactive: err %w, output %s", launcherPath, err, string(out))
	}

	results := make(osqResp, 0)
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parsing %s interactive: err %w, output %s", launcherPath, err, string(out))
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("got empty response from %s interactive: output %s", launcherPath, string(out))
	}

	return results[0], nil
}

func (hc *hostInfoCheckup) bboltDbSize() string {
	db := hc.k.BboltDB()
	if db == nil {
		return "error: bbolt db connection was not available via knapsack"
	}

	boltStats, err := agent.GetStats(db)
	if err != nil {
		return fmt.Sprintf("encountered error accessing bbolt stats: %w", err.Error())
	}

	return strconv.FormatInt(boltStats.DB.Size, 10)
}

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("ERROR: %s", err)
	}

	return hostname
}

func agentKeyInfo() map[string]string {
	keyinfo := make(map[string]string, 3)

	pub := agent.LocalDbKeys().Public()
	if pub == nil {
		keyinfo["local_key"] = "nil. Likely startup delay"
		return keyinfo
	}

	if localKeyDer, err := x509.MarshalPKIXPublicKey(pub); err == nil {
		// der is a binary format, so convert to b64
		keyinfo["local_key"] = base64.StdEncoding.EncodeToString(localKeyDer)
	} else {
		keyinfo["local_key"] = fmt.Sprintf("error marshalling local key (startup is sometimes weird): %s", err)
	}

	// We don't always have hardware keys. Move on if we don't
	if agent.HardwareKeys().Public() == nil {
		return keyinfo
	}

	if hardwareKeyDer, err := x509.MarshalPKIXPublicKey(agent.HardwareKeys().Public()); err == nil {
		// der is a binary format, so convert to b64
		keyinfo["hardware_key"] = base64.StdEncoding.EncodeToString(hardwareKeyDer)
		keyinfo["hardware_key_source"] = agent.HardwareKeys().Type()
	}

	return keyinfo
}
