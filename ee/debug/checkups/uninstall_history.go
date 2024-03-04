package checkups

import (
	"context"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
)

type (
	uninstallHistoryCheckup struct {
		k       types.Knapsack
		status  Status
		summary string
		data    map[string]any
	}
)

func (hc *uninstallHistoryCheckup) Data() any             { return hc.data }
func (hc *uninstallHistoryCheckup) ExtraFileName() string { return "" }
func (hc *uninstallHistoryCheckup) Name() string          { return "Uninstall History" }
func (hc *uninstallHistoryCheckup) Status() Status        { return hc.status }
func (hc *uninstallHistoryCheckup) Summary() string       { return hc.summary }

func (hc *uninstallHistoryCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	hc.data = make(map[string]any)

	return nil
}
