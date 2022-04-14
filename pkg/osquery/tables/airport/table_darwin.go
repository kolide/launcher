//go:build darwin
// +build darwin

package airport

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

var (
	allowedOptions = []string{"getinfo", "scan"}
	airportPaths   = []string{"/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"}
)

type Table struct {
	name   string
	logger log.Logger
}

func TablePlugin(_ *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("option"),
	)

	t := &Table{
		name:   "kolide_airport_util",
		logger: logger,
	}

	return table.NewPlugin(t.name, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

	options := tablehelpers.GetConstraints(queryContext, "option", tablehelpers.WithAllowedValues(allowedOptions))

	if len(options) == 0 {
		return nil, errors.Errorf("The %s table requires that you specify a constraint for option", t.name)
	}

	if len(options) > 1 {
		return nil, errors.Errorf("The %s table only supports one constraint for option", t.name)
	}

	option := options[0]

	airportOutput, err := tablehelpers.Exec(ctx, t.logger, 30, airportPaths, []string{"--" + option})
	if err != nil {
		level.Debug(t.logger).Log("msg", "Error execing airport", "option", option, "err", err)
		return nil, err
	}

	outputReader := bytes.NewReader(airportOutput)

	return processAirportOutput(outputReader, option, queryContext, t.logger)
}

func processAirportOutput(airportOutput io.Reader, option string, queryContext table.QueryContext, logger log.Logger) ([]map[string]string, error) {
	var results []map[string]string

	var unmarshalledOutput []map[string]interface{}
	rowData := map[string]string{"option": option}

	switch option {
	case "getinfo":
		unmarshalledOutput = []map[string]interface{}{unmarshallGetInfoOutput(airportOutput)}
	case "scan":
		unmarshalledOutput = unmarshallScanOuput(airportOutput)
	default:
		return nil, errors.Errorf("unsupported option %s", option)
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

		flattened, err := dataflatten.Flatten(unmarshalledOutput, dataflatten.WithLogger(logger), dataflatten.WithQuery(strings.Split(dataQuery, "/")))
		if err != nil {
			return nil, err
		}
		results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
	}

	return results, nil
}

// unmarshallScanOuput parses the output of the airport getinfo command
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

	result := make(map[string]interface{})

	for scanner.Scan() {
		line := scanner.Text()

		if len(line) == 0 {
			continue
		}

		parts := strings.Split(line, ": ")
		key := strings.TrimSpace(parts[0])

		if key == "" {
			continue
		}

		if len(parts) == 1 {
			// if there is not value the key will retain the : at the end
			// so remove it if that is the case
			key = key[:len(key)-1]
			result[key] = nil
			continue
		}

		value := strings.TrimSpace(parts[1])

		result[key] = value
	}

	return result
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

		if len(line) == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		if headerSeparatorIndexes == nil {
			headerSeparatorIndexes = columnSeparatorIndexes(line)

			if len(headerSeparatorIndexes) > 1 {
				headerSeparatorIndexes[len(headerSeparatorIndexes)-1] = headerSeparatorIndexes[len(headerSeparatorIndexes)-1] + headerSeparatorIndexes[len(headerSeparatorIndexes)-2]
				headerSeparatorIndexes = headerSeparatorIndexes[:len(headerSeparatorIndexes)-1]
			}

			headers = trimSpaces(splitAtIndexes(line, headerSeparatorIndexes))

			continue
		}

		rowData := make(map[string]interface{}, len(headers))
		rowValues := trimSpaces(splitAtIndexes(line, headerSeparatorIndexes))

		for i, v := range rowValues {
			rowData[headers[i]] = v
		}

		results = append(results, rowData)
	}

	return results
}

func trimSpaces(strs []string) []string {
	var trimmed []string
	for _, str := range strs {
		trimmed = append(trimmed, strings.TrimSpace(str))
	}
	return trimmed
}

func splitAtIndexes(str string, indexes []int) []string {
	var result []string

	startIndex := 0
	for _, index := range indexes {
		result = append(result, str[startIndex:index])
		startIndex = index
	}

	result = append(result, str[startIndex:])

	return result
}

func columnSeparatorIndexes(line string) []int {

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

	return indexes
}
