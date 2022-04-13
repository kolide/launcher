//go:build darwin
// +build darwin

package airport

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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

// unmarshallScanOuput parses the output of the airport scan command
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
			result[key] = ""
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

	// use this to store the cells of the header row inclucing whitespaces
	// we can use the length of these to determine the column widths
	// we need this so we can handle whitespaces in SSID and Security column
	var rawHeaders []string
	var results []map[string]interface{}

	for scanner.Scan() {
		line := scanner.Text()

		if len(line) == 0 || strings.Trim(line, " ") == "" {
			continue
		}

		// first popoulate the headers including spaces
		// this also determines the column widths
		if rawHeaders == nil {
			rawHeaders = rawSpaceSeparatedWords(line)

			if len(rawHeaders) < 2 {
				continue
			}

			// the last header: "SECURITY (auth/unicast/group)" has a space in it, so combine them
			rawHeaders[len(rawHeaders)-2] = fmt.Sprintf("%s %s", rawHeaders[len(rawHeaders)-2], rawHeaders[len(rawHeaders)-1])
			rawHeaders = rawHeaders[:len(rawHeaders)-1]
			continue
		}

		rowData := make(map[string]interface{})

		startRuneIndex := 0
		for headerIndex, header := range rawHeaders {

			key := strings.Trim(header, " ")

			endRuneIndex := startRuneIndex + len(header)

			// if last header or runes left are less than what column should be, use remainder of line
			// for example: WPA(PSK/AES,TKIP/TKIP) RSN(PSK/AES,TKIP/TKIP)
			if headerIndex == len(rawHeaders)-1 || len(line) < endRuneIndex {
				rowData[key] = strings.Trim(line[startRuneIndex:], " ")
				break
			}

			value := strings.Trim(line[startRuneIndex:endRuneIndex], " ")
			// trim whitespaces from header value
			rowData[key] = value
			startRuneIndex = endRuneIndex + 1
		}

		results = append(results, rowData)
	}

	return results
}

func rawSpaceSeparatedWords(line string) []string {
	separatorIndexes := columnSeparatorIndexes(line)

	var words []string
	lastSeparatorIndex := 0

	for _, separatorIndex := range separatorIndexes {
		words = append(words, line[lastSeparatorIndex:separatorIndex])
		lastSeparatorIndex = separatorIndex + 1
	}

	words = append(words, line[lastSeparatorIndex:])

	return words
}

func columnSeparatorIndexes(line string) []int {
	var indexes []int

	lastSeperatorIndex := 0

	for {
		trimmedLeft := strings.TrimLeft(line, " ")

		endIndex := indexOfLastSpaceBeforeNonSpace(trimmedLeft)
		if endIndex == -1 {
			return indexes
		}

		leadingSpacesCount := len(line) - len(trimmedLeft)

		separatorIndex := lastSeperatorIndex + leadingSpacesCount + endIndex
		indexes = append(indexes, separatorIndex)

		lastSeperatorIndex = separatorIndex

		line = trimmedLeft[endIndex:]
	}
}

func indexOfLastSpaceBeforeNonSpace(str string) int {
	for i := 0; i < len(str)-1; i++ {
		if str[i] == ' ' && str[i+1] != ' ' {
			return i
		}
	}

	return -1
}
