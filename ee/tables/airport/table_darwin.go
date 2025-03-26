//go:build darwin
// +build darwin

package airport

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

var (
	allowedOptions = []string{"getinfo", "scan"}
)

type Table struct {
	name    string
	slogger *slog.Logger
}

const tableName = "kolide_airport_util"

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("option"),
	)

	t := &Table{
		name:    tableName,
		slogger: slogger.With("name", tableName),
	}

	return tablewrapper.New(flags, slogger, t.name, columns, t.generate)
}

type airportExecutor struct {
	ctx     context.Context // nolint:containedctx
	slogger *slog.Logger
}

func (a *airportExecutor) Exec(option string) ([]byte, error) {
	return tablehelpers.RunSimple(a.ctx, a.slogger, 30, allowedcmd.Airport, []string{"--" + option})
}

type executor interface {
	Exec(string) ([]byte, error)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", tableName)
	defer span.End()

	airportExecutor := &airportExecutor{
		ctx:     ctx,
		slogger: t.slogger,
	}

	return generateAirportData(ctx, queryContext, airportExecutor, t.slogger)
}

func generateAirportData(ctx context.Context, queryContext table.QueryContext, airportExecutor executor, slogger *slog.Logger) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	options := tablehelpers.GetConstraints(queryContext, "option", tablehelpers.WithAllowedValues(allowedOptions))

	if len(options) == 0 {
		return nil, fmt.Errorf("The %s table requires that you specify a constraint for WHERE option. Valid values for option are (%s).", tableName, strings.Join(allowedOptions, ", "))
	}

	var results []map[string]string
	for _, option := range options {
		airportOutput, err := airportExecutor.Exec(option)
		if err != nil {
			slogger.Log(ctx, slog.LevelDebug,
				"error execing airport",
				"option", option,
				"err", err,
			)
			continue
		}

		optionResult, err := processAirportOutput(bytes.NewReader(airportOutput), option, queryContext, slogger)
		if err != nil {
			slogger.Log(ctx, slog.LevelDebug,
				"error processing airport output",
				"option", option,
				"err", err,
			)
			continue
		}
		results = append(results, optionResult...)
	}

	return results, nil
}

func processAirportOutput(airportOutput io.Reader, option string, queryContext table.QueryContext, slogger *slog.Logger) ([]map[string]string, error) {
	var results []map[string]string

	var unmarshalledOutput []map[string]interface{}

	rowData := map[string]string{"option": option}

	switch option {
	case "getinfo":
		unmarshalledOutput = []map[string]interface{}{unmarshallGetInfoOutput(airportOutput)}
	case "scan":
		unmarshalledOutput = unmarshallScanOuput(airportOutput)
	default:
		return nil, fmt.Errorf("unsupported option %s", option)
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

		flattened, err := dataflatten.Flatten(unmarshalledOutput, dataflatten.WithSlogger(slogger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			return nil, err
		}
		results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return results, nil
}

// unmarshallGetInfoOutput parses the output of the airport getinfo command
func unmarshallGetInfoOutput(reader io.Reader) map[string]interface{} {
	/* example output:

	    agrCtlRSSI: -55
	    agrExtRSSI: 0
	   agrCtlNoise: -94
	   agrExtNoise: 0
	         state: running
	       op mode: station
	       ...
	*/
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	results := make(map[string]interface{})

	for scanner.Scan() {
		line := scanner.Text()

		// since BSSID (a1:a1:a1...) contains ":", we need to split on ": "
		parts := trimSpaces(strings.Split(line, ": "))

		// if we only have a key, move on
		if len(parts) < 2 {
			continue
		}

		results[parts[0]] = parts[1]
	}

	return results
}

// unmarshallScanOuput parses the output of the airport scan command
func unmarshallScanOuput(reader io.Reader) []map[string]interface{} {
	/* example output:

	            SSID BSSID             RSSI CHANNEL HT CC SECURITY (auth/unicast/group)
	   i got spaces! a0:a0:a0:a0:a0:a0 -92  108     Y  US WPA(PSK/AES,TKIP/TKIP) RSN(PSK/AES,TKIP/TKIP)
	       no-spaces b1:b1:b1:b1:b1:b1 -91  116     N  EU RSN(PSK/AES/AES)
	*/

	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	var headers []string
	var headerSeparatorIndexes []int
	var results []map[string]interface{}

	for scanner.Scan() {
		line := scanner.Text()

		if headerSeparatorIndexes == nil {
			headerSeparatorIndexes = scanColumnSeparatorIndexes(line)
			headers = splitAtIndexes(line, headerSeparatorIndexes)
			continue
		}

		rowData := make(map[string]interface{}, len(headers))

		for i, value := range splitAtIndexes(line, headerSeparatorIndexes) {
			rowData[headers[i]] = value
		}

		results = append(results, rowData)
	}

	return results
}

func splitAtIndexes(str string, indexes []int) []string {
	var result []string

	startIndex := 0
	for _, index := range indexes {

		// if the index is out of bounds, return the rest of the string
		if index >= len(str) {
			result = append(result, str[startIndex:])
			return result
		}

		result = append(result, str[startIndex:index])
		startIndex = index
	}

	result = append(result, str[startIndex:])

	return trimSpaces(result)
}

func trimSpaces(strs []string) []string {
	for i, str := range strs {
		strs[i] = strings.TrimSpace(str)
	}
	return strs
}

func scanColumnSeparatorIndexes(line string) []int {

	/* example header row of airport scan:

	        SSID BSSID             RSSI CHANNEL HT CC SECURITY (auth/unicast/group)

	the leading spaces that are tricky
	the first column "SSID" is right aligned
	then the next column "BSSID" is left aligned
	*/

	var indexes []int
	foundFirstWord := false

	for i := 0; i < len(line)-1; i++ {
		if line[i] == ' ' && line[i+1] != ' ' {

			// due to leading spaces, don't record the index until the second time we meed the condition
			if !foundFirstWord {
				foundFirstWord = true
				continue
			}
			indexes = append(indexes, i)
		}
	}

	// since the last column has a space in it, just remove the last index
	if len(indexes) > 0 {
		indexes = indexes[:len(indexes)-1]
	}

	return indexes
}
