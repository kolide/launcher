package checkups

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"

	"github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/types"
)

type (
	hostInfoCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (hc *hostInfoCheckup) Data() map[string]any  { return hc.data }
func (hc *hostInfoCheckup) ExtraFileName() string { return "" }
func (hc *hostInfoCheckup) Name() string          { return "Host Info" }
func (hc *hostInfoCheckup) Status() Status        { return hc.status }
func (hc *hostInfoCheckup) Summary() string       { return hc.summary }

func (hc *hostInfoCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	hc.data = make(map[string]any)
	hc.data["hostname"] = hostName()
	hc.data["keyinfo"] = agentKeyInfo()
	hc.data["bbolt_db_size"] = hc.bboltDbSize()
	desktopProcesses := runner.InstanceDesktopProcessRecords()
	hc.data["user_desktop_processes"] = desktopProcesses

	if runtime.GOOS == "windows" {
		hc.data["in_modern_standby"] = hc.k.InModernStandby()
	}

	hc.status = Informational
	hc.summary = fmt.Sprintf("hostname: %s", hc.data["hostname"])

	return nil
}

func (hc *hostInfoCheckup) bboltDbSize() string {
	db := hc.k.BboltDB()
	if db == nil {
		return "error: bbolt db connection was not available via knapsack"
	}

	boltStats, err := agent.GetStats(db)
	if err != nil {
		return fmt.Sprintf("encountered error accessing bbolt stats: %s", err.Error())
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
		keyinfo["local_key"] = fmt.Sprintf("error marshalling local key (startup is sometimes weird): %s", err.Error())
	}

	// We don't always have hardware keys. Move on if we don't
	if agent.HardwareKeys().Public() == nil {
		return keyinfo
	}

	if hardwareKeyDer, err := x509.MarshalPKIXPublicKey(agent.HardwareKeys().Public()); err == nil {
		// der is a binary format, so convert to b64
		keyinfo["hardware_key"] = base64.StdEncoding.EncodeToString(hardwareKeyDer)
		keyinfo["hardware_key_source"] = agent.HardwareKeys().Type()
	} else {
		keyinfo["hardware_key"] = fmt.Sprintf("error marshalling hardware key: %s", err.Error())
	}

	return keyinfo
}
