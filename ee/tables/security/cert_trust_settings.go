//go:build darwin
// +build darwin

package security

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type certTrustSettingsTable struct {
	name    string
	slogger *slog.Logger
}

const (
	tableName = "kolide_certificate_trust"
)

var allowedDomains = []string{
	"admin",
	"system",
}

func CertTrustSettingsTablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("domain"),
	)

	c := &certTrustSettingsTable{
		name:    tableName,
		slogger: slogger.With("name", tableName),
	}

	return tablewrapper.New(flags, slogger, c.name, columns, c.generate)
}

func (c *certTrustSettingsTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	_, span := observability.StartSpan(ctx, "table_name", tableName)
	defer span.End()

	// We expect a domain to be provided, and that the domain is either "system" or "admin".
	domains := tablehelpers.GetConstraints(queryContext, "domain", tablehelpers.WithAllowedValues(allowedDomains))
	if len(domains) == 0 {
		return nil, fmt.Errorf("the %s table requires that you specify a constraint for WHERE domain; valid values for domain are (%s)", tableName, strings.Join(allowedDomains, ", "))
	}

	results := make([]map[string]string, 0)

	// Since this is a dataflatten table, check for `query` constraints.
	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		for _, domain := range domains {
			// Create the `security` command and run it
			args := []string{"dump-trust-settings"}
			switch domain {
			case "system":
				args = append(args, "-s")
			case "admin":
				args = append(args, "-d")
			default:
				// Should never make it here due to constraint check above in `GetConstraints` / `WithAllowedValues`
				return nil, fmt.Errorf("unsupported domain %s", domain)
			}

			var output bytes.Buffer
			var stderr bytes.Buffer
			if err := tablehelpers.Run(ctx, c.slogger, 10, allowedcmd.Security, args, &output, &stderr); err != nil {
				if strings.Contains(stderr.String(), noCerts) {
					// No certs for this domain -- no rows to add here.
					continue
				}
				return nil, fmt.Errorf("running security dump-trust-settings: got output `%s` and stderr `%s`: %w", output.String(), stderr.String(), err)
			}

			// Parse the trusted certs from the output
			trustedCerts, err := parseTrustSettingsDump(&output)
			if err != nil {
				return nil, fmt.Errorf("parsing security dump-trust-settings output: %w", err)
			}

			// Marshal the trusted certs to JSON for dataflattening, then dataflatten.
			rawTrustedCerts, err := json.Marshal(trustedCerts)
			if err != nil {
				return nil, fmt.Errorf("marshalling trusted certs for dataflattening: %w", err)
			}
			flattened, err := dataflatten.Json(rawTrustedCerts, []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(c.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}...)
			if err != nil {
				c.slogger.Log(ctx, slog.LevelWarn,
					"could not flatten trusted certs",
					"err", err,
				)
				continue
			}

			rowData := map[string]string{
				"domain": domain,
			}
			results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
		}
	}

	return results, nil
}

type (
	trustedCert struct {
		CertName      string          `json:"cert_name"`
		TrustSettings []*trustSetting `json:"trust_settings"`
	}

	trustSetting struct {
		PolicyOID    string `json:"policy_oid"`
		AllowedError string `json:"allowed_error"`
		ResultType   string `json:"result_type"`
	}
)

var (
	lineDelimiter             byte = '\n'
	noCerts                        = "No Trust Settings were found"
	outputHeaderRegexp             = regexp.MustCompile(`^Number of trusted certs\s*=\s*(\d+)$`)  // Matches `Number of trusted certs = 1`
	certHeaderPrefix               = "Cert "                                                      // Looks like: `Cert 0: Example Root CA`
	trustSettingsHeaderRegexp      = regexp.MustCompile(`^Number of trust settings\s*:\s*(\d+)$`) // Matches `Number of trust settings : 10`
	trustSettingHeaderRegexp       = regexp.MustCompile(`^Trust Setting (\d+):$`)                 // Matches `Trust Setting 0:`
	trustSettingHeaderPrefix       = "Trust Setting"                                              // Looks like: `Trust Setting 2:`
)

// parseTrustSettingsDump parses the results of running `security dump-trust-settings -s` or
// `security dump-trust-settings -d`.
func parseTrustSettingsDump(dump *bytes.Buffer) ([]*trustedCert, error) {
	reader := bufio.NewReader(dump)

	// The very first line should tell us how many certs to expect. It will match `outputHeaderRegex`
	// and indicate how many certs we will need to read. It looks like "Number of trusted certs = 7".
	firstLine, err := reader.ReadString(lineDelimiter)
	if err != nil {
		return nil, fmt.Errorf("reading first line of output: %w", err)
	}
	firstLine = strings.TrimSpace(firstLine)

	// Extract the number of expected certs.
	matches := outputHeaderRegexp.FindAllStringSubmatch(firstLine, -1)
	if len(matches) < 1 || len(matches[0]) < 2 {
		// Check for error string indicating there are no trust settings.
		// Typically, we would expect to have already parsed this from stderr,
		// but we check here for completeness.
		if strings.Contains(firstLine, noCerts) {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing output header: %s", firstLine)
	}
	expectedCertCountStr := matches[0][1]
	expectedCertCount, err := strconv.Atoi(expectedCertCountStr)
	if err != nil {
		return nil, fmt.Errorf("parsing number of certs: %w", err)
	}

	// Read the expected number of certs from the reader.
	trustedCerts := make([]*trustedCert, expectedCertCount)
	for i := range expectedCertCount {
		cert, err := parseCert(reader)
		if err != nil {
			return nil, fmt.Errorf("parsing cert %d of %d: %w", i, expectedCertCount, err)
		}
		trustedCerts[i] = cert
	}

	return trustedCerts, nil
}

// parseCert parses the upcoming cert from the reader. The cert takes the following format:
//
//	Cert 0: Example Root CA
//	   Number of trust settings : 1
//	   Trust Setting 0:
//	      Policy OID            : SSL
//	      Allowed Error         : CSSMERR_TP_CERT_EXPIRED
//	      Result Type           : kSecTrustSettingsResultTrustRoot
//
// If there are no trust settings, the second line will instead read `Number of trust settings : 0`,
// and the trust settings list will be omitted:
//
//	Cert 0: Example Root CA
//	   Number of trust settings : 0
func parseCert(reader *bufio.Reader) (*trustedCert, error) {
	currentLine, err := reader.ReadString(lineDelimiter)
	if err != nil {
		return nil, fmt.Errorf("reading first line of cert: %w", err)
	}
	currentLine = strings.TrimSpace(currentLine)

	// We expect the first line to have prefix `Cert `
	if !strings.HasPrefix(currentLine, certHeaderPrefix) {
		return nil, fmt.Errorf("malformed cert header: %s", currentLine)
	}

	// Extract the cert name from the current line. The line will look like `Cert 0: Example Root CA`,
	// so we split on the colon and take the second part of the string as the cert name.
	certParts := strings.SplitN(currentLine, ":", 2)
	if len(certParts) != 2 {
		return nil, fmt.Errorf("malformed cert header: %s", currentLine)
	}
	cert := trustedCert{
		CertName: strings.TrimSpace(certParts[1]),
	}

	// Parse any trust settings associated with this cert.
	trustSettings, err := parseTrustSettings(reader)
	if err != nil {
		return nil, fmt.Errorf("could not parse trust settings for cert %s: %w", cert.CertName, err)
	}
	cert.TrustSettings = trustSettings

	return &cert, nil
}

// parseTrustSettings parses the upcoming list of trust settings from the reader.
// The trust settings list takes the following format:
//
//	 Number of trust settings : 2
//		    Trust Setting 0:
//			      <data here>
//		    Trust Setting 1:
//		       <data here>
//
// parseTrustSettings will extract the number of trust settings from the first line, and then
// parse that number of trust settings.
func parseTrustSettings(reader *bufio.Reader) ([]*trustSetting, error) {
	// Read the first line, which will be a header matching `trustSettingsHeaderRegexp` indicating
	// how many trust settings this cert has.
	currentLine, err := reader.ReadString(lineDelimiter)
	if err != nil {
		return nil, fmt.Errorf("reading first line of trust settings: %w", err)
	}
	currentLine = strings.TrimSpace(currentLine)

	// Extract the number of trust settings from the line.
	matches := trustSettingsHeaderRegexp.FindAllStringSubmatch(currentLine, -1)
	if len(matches) < 1 || len(matches[0]) < 2 {
		return nil, fmt.Errorf("no trust settings: %s", currentLine)
	}
	expectedTrustSettingsCountStr := matches[0][1]
	expectedTrustSettingsCount, err := strconv.Atoi(expectedTrustSettingsCountStr)
	if err != nil {
		return nil, fmt.Errorf("parsing number of trust settings: %w", err)
	}

	// Parse the expected number of trust settings from our reader.
	trustSettings := make([]*trustSetting, expectedTrustSettingsCount)
	for i := range expectedTrustSettingsCount {
		trustSetting, err := parseTrustSetting(reader)
		if err != nil {
			return nil, fmt.Errorf("parsing trust setting %d of %d: %w", i, expectedTrustSettingsCount, err)
		}
		trustSettings[i] = trustSetting
	}

	return trustSettings, nil
}

// parseTrustSetting parses the upcoming trust setting from the given reader.
// The trust setting, in context, will look like this:
//
//	Cert 0: Example Root CA
//	   Number of trust settings : 1
//	   Trust Setting 0:
//	      Policy OID            : SSL
//	      Allowed Error         : CSSMERR_TP_CERT_EXPIRED
//	      Result Type           : kSecTrustSettingsResultTrustRoot
//	Cert 1: ...
//
// Here, we attempt to parse Policy OID, Allowed Error, and Result Type. The function terminates
// when it peeks ahead and sees either another trust setting (Trust Setting 1) or another cert (Cert 1: ...).
func parseTrustSetting(reader *bufio.Reader) (*trustSetting, error) {
	// Read the first line, which will be a header matching `trustSettingHeaderRegexp`
	// indicating our current index (e.g. "Trust Setting 0".)
	currentLine, err := reader.ReadString(lineDelimiter)
	if err != nil {
		return nil, fmt.Errorf("reading first line of trust setting: %w", err)
	}
	currentLine = strings.TrimSpace(currentLine)

	// We expect the first line to match `trustSettingHeaderRegexp``. We don't actually need
	// to extract the index here, but we want to make sure that we haven't gotten off with parsing
	// anywhere.
	matches := trustSettingHeaderRegexp.FindAllStringSubmatch(currentLine, -1)
	if len(matches) < 1 || len(matches[0]) < 2 {
		return nil, fmt.Errorf("no trust setting: %s", currentLine)
	}

	// Read trust setting info, peeking ahead first to see whether we've completed reading this setting.
	t := trustSetting{}
	for {
		// We peek ahead here to see whether we've already finished reading this setting.
		// We check for a line starting with `certHeaderPrefix` (indicating this was the last
		// trust setting in our list of trust settings, and we've advanced to the next cert), or
		// `trustSettingHeaderPrefix` (indicating this was not the last trust setting in our list, and
		// we've advanced to the next trust setting). The longer of these strings is `trustSettingHeaderPrefix`,
		// typically with 3 spaces before it. So, we peek ahead len(trustSettingHeaderPrefix)+10, to be safe.
		nextBytes, err := reader.Peek(len(trustSettingHeaderPrefix) + 10)
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) && !errors.Is(err, io.EOF) {
			// If the error is io.EOF, we don't want to return yet -- we might have another line left
			// to read that is under `len(trustSettingHeaderPrefix) + 10` bytes in length.
			return nil, fmt.Errorf("peeking ahead: %w", err)
		}
		nextStr := strings.TrimSpace(string(nextBytes))
		if strings.HasPrefix(nextStr, certHeaderPrefix) || strings.HasPrefix(nextStr, trustSettingHeaderPrefix) {
			// The next line is either a new cert or a new trust setting -- we're done reading this one!
			break
		}

		// We've confirmed we have more of this current trust setting to read -- read in the next line.
		currentLine, err := reader.ReadString(lineDelimiter)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// We've reached the end of our input -- return our trust setting.
				break
			}
			return nil, fmt.Errorf("reading next line of trust setting: %w", err)
		}

		// The trust setting data is a colon-delimited key-value pair. Split on the colon
		// to extract the key and value.
		parts := strings.SplitN(currentLine, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed setting in trust setting: `%s`", currentLine)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Policy OID":
			t.PolicyOID = value
		case "Allowed Error":
			t.AllowedError = value
		case "Result Type":
			t.ResultType = value
		}
	}

	return &t, nil
}
