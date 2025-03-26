//go:build windows
// +build windows

package table

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/dsim_default_associations"
	"github.com/kolide/launcher/ee/tables/execparsers/dsregcmd"
	"github.com/kolide/launcher/ee/tables/secedit"
	"github.com/kolide/launcher/ee/tables/wifi_networks"
	"github.com/kolide/launcher/ee/tables/windowsupdatetable"
	"github.com/kolide/launcher/ee/tables/wmitable"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func BenchmarkProgramIcons(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	programIconsTable := ProgramIcons(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := programIconsTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkDsimDefaultAssocations(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	dsimDefaultAssociationsTable := dsim_default_associations.TablePlugin(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := dsimDefaultAssociationsTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkSeceditTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	seceditTable := secedit.TablePlugin(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := seceditTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWifiNetworksTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	wifiNetworksTable := wifi_networks.TablePlugin(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := wifiNetworksTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWindowsUpdatesTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	updatesTable := windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := updatesTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWindowsHistoryTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	historyTable := windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := historyTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWmiTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	wmiTable := wmitable.TablePlugin(mockFlags, slogger)

	/*
		type queryContextJSON struct {
			Constraints []constraintListJSON `json:"constraints"`
		}

		type constraintListJSON struct {
			Name     string          `json:"name"`
			Affinity string          `json:"affinity"`
			List     json.RawMessage `json:"list"`
		}
	*/
	classConstraint := map[string]string{
		"op":   "2", // table.OperatorEquals
		"expr": "SoftwareLicensingProduct",
	}
	classConstraintRaw, err := json.Marshal(classConstraint)
	require.NoError(b, err)
	propertiesConstraint := map[string]string{
		"op":   "2", // equals
		"expr": "name,licensefamily,id,licensestatus,licensestatusreason,genuinestatus,partialproductkey,productkeyid",
	}
	propertiesConstraintRaw, err := json.Marshal(propertiesConstraint)
	require.NoError(b, err)
	constraintsMap := map[string]any{
		"constraints": []map[string]any{
			{
				"name":     "class",
				"affinity": "TEXT",
				"list":     classConstraintRaw,
			},
			{
				"name":     "properties",
				"affinity": "TEXT",
				"list":     propertiesConstraintRaw,
			},
		},
	}
	queryContextStr, err := json.Marshal(constraintsMap)
	require.NoError(b, err)

	for range b.N {
		// Confirm we can call the table successfully
		response := wmiTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": string(queryContextStr),
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkDsregcmd(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	dsregcmdTable := dataflattentable.NewExecAndParseTable(mockFlags, slogger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`})

	for range b.N {
		// Confirm we can call the table successfully
		response := dsregcmdTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}
