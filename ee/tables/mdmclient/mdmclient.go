//go:build darwin
// +build darwin

// Package mdmclient provides a table that parses the mdmclient
// output. Empirically, this seems to be an almost gnustep
// plist. With some headers. So, unwind that.
package mdmclient

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// headerRegex matches the header that may be included at the beginning of the mdmclient response,
// which takes the following format:
//
//	=== CPF_GetInstalledProfiles === (<Device>)
//	Number of <Device> profiles found: 6 (Filtered: 0)
var headerRegex = regexp.MustCompile(`^=== CPF_GetInstalledProfiles === \(<Device>\)\nNumber of <Device> profiles found: \d+ \(Filtered: \d+\)\n`)

// lengthBytesRegex matches entries like the PushToken or SignerCertificates entries, which we have to
// manually adjust to be parseable as a plist. The first capture group gets the length property with
// a comma after it, which we must replace with a semicolon. The second capture group gets the bytes
// property, which needs to be quoted and have a semicolon appended.
//
// The PushToken line takes the following format:
//
// PushToken = {length = 32, bytes = 0x068b4535 172f7bd3 851facee c98e0d88 ... 38625271 61731ac3 };
//
// The SignerCertificates block takes the following format:
//
//	SignerCertificates =             (
//		{length = 1402, bytes = 0x30820576 3082045e a0030201 02020809 ... afb8b2d1 abcdabcd },
//		{length = 1052, bytes = 0x30820418 30820300 a0030201 02020804 ... 26cffc17 abcdabcd }
//	);
var lengthBytesRegex = regexp.MustCompile(`{length = (\d+,) bytes = (0[xX][0-9a-fA-F\.\s]+)}`)

type Table struct {
	logger    log.Logger
	tableName string
}

func TablePlugin(logger log.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("command"),
	)

	t := &Table{
		logger:    logger,
		tableName: "kolide_mdmclient",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	gcOpts := []tablehelpers.GetConstraintOpts{
		tablehelpers.WithAllowedCharacters(allowedCharacters),
		tablehelpers.WithLogger(t.logger),
		tablehelpers.WithDefaults(""),
	}

	for _, mdmclientCommand := range tablehelpers.GetConstraints(queryContext, "command", gcOpts...) {
		if mdmclientCommand == "" {
			level.Info(t.logger).Log("msg", "command must not be blank")
			continue
		}

		if !strings.HasPrefix(mdmclientCommand, "Query") {
			level.Info(t.logger).Log("msg", "Only Query commands are supported")
			continue
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {

			mdmclientOutput, err := tablehelpers.Exec(ctx, t.logger, 30, allowedcmd.Mdmclient, []string{mdmclientCommand}, false)
			if err != nil {
				level.Info(t.logger).Log("msg", "mdmclient failed", "err", err)
				continue
			}

			flatData, err := t.flattenOutput(dataQuery, mdmclientOutput)
			if err != nil {
				level.Info(t.logger).Log("msg", "flatten failed", "err", err)
				continue
			}

			rowData := map[string]string{
				"command": mdmclientCommand,
			}

			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
		}
	}
	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, systemOutput []byte) ([]dataflatten.Row, error) {
	converted, err := t.transformOutput(systemOutput)
	if err != nil {
		level.Info(t.logger).Log("msg", "converting mdmclient output", "err", err)
		return nil, fmt.Errorf("converting: %w", err)
	}

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(t.logger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	return dataflatten.Plist(converted, flattenOpts...)
}

// transformOutput has some hackish rules to transform the output into a "proper" gnustep plist
func (t *Table) transformOutput(in []byte) ([]byte, error) {
	out := headerRegex.ReplaceAll(in, []byte{})

	// We can't access the agent response when running launcher normally -- we get the error
	// "[ERROR] Unable to target 'local user' via XPC when running as daemon". In that case,
	// remove the null agent response.
	out = bytes.Replace(out, []byte("Agent response: (null)\n"), []byte{}, 1)

	out = bytes.Replace(out, []byte("Daemon response: {"), []byte("DaemonResponse = {"), 1)
	out = bytes.Replace(out, []byte("Agent response: {"), []byte("AgentResponse = {"), 1)

	// This would, honestly, be cleaner as a regex. The \n aren't
	// quite right. We want to replace any unindented } with a
	// };. Which is just a hack, because we really want to replace
	// the one that matches the response structures.
	out = bytes.Replace(out, []byte("\n}\n"), []byte("\n};\n"), 2)

	// Adjust the PushToken and SignerCertificates entries, if present
	out = transformLengthByteEntriesInOutput(out)

	var retOut []byte
	retOut = append(retOut, "{\n"...)
	retOut = append(retOut, out...)
	retOut = append(retOut, "\n}\n"...)
	return retOut, nil
}

// transformLengthByteEntriesInOutput adjusts the formatting of nested blocks with length/byte properties to be
// parseable in a plist.
//
// Example --
// original: `PushToken = {length = 32, bytes = 0x068b4535 172f7bd3 851facee c98e0d88 ... 38625271 61731ac3 };`
// transformed: `PushToken = {length = 32; bytes = "0x068b4535 172f7bd3 851facee c98e0d88 ... 38625271 61731ac3 "; };`
func transformLengthByteEntriesInOutput(out []byte) []byte {
	matches := lengthBytesRegex.FindAllSubmatchIndex(out, -1)

	if len(matches) == 0 {
		return out
	}

	// Iterate backwards through matches to avoid messing up indices for earlier
	// matches when performing insertions.
	for i := len(matches) - 1; i >= 0; i -= 1 {
		match := matches[i]
		// First two items in `match` are start/end indices for entire line; second two items are
		// start/end indices for `length` property (the first capture group); third two items are
		// start/end indices for `bytes` property (the second capture group).
		if len(match) != 6 {
			continue
		}

		// Replace comma with semicolon for first capture group (e.g. transforming `length = 32,` to `length = 32;`)
		lengthEndIndex := match[3]
		out[lengthEndIndex-1] = ';'

		// Quote second capture group and append a semicolon (e.g., transforming
		// `bytes = 0x068b4535 172f7bd3 851facee c98e0d88 ... 38625271 61731ac3 ` to
		// `bytes = "0x068b4535 172f7bd3 851facee c98e0d88 ... 38625271 61731ac3";`)
		bytesStartIndex := match[4]
		bytesEndIndex := match[5]

		// Insert opening quote mark
		out = insertAt(out, bytesStartIndex, '"')

		// Insert closing quote mark after `bytes`
		out = insertAt(out, bytesEndIndex+1, '"')

		// Insert semicolon after previous insertion point
		out = insertAt(out, bytesEndIndex+2, ';')
	}

	return out
}

func insertAt(original []byte, insertIndex int, value byte) []byte {
	original = append(original[:insertIndex+1], original[insertIndex:]...)
	original[insertIndex] = value

	return original
}
