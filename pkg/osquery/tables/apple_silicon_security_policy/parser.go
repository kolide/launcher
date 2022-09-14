package apple_silicon_security_policy

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

// This regexp gets matches for an arbitrary property name, and a four-character code (4CC) sequence
var fourCharCodeRegexp = regexp.MustCompile(`^((.*) (?:\((.{4})\)))`)

// This regexp gets matches for the volume group UUID following the text "Local policy for volume group"
var volumeGroupRegexp = regexp.MustCompile("^((?:(?:.*) for volume group))(.*):")

// parseStatus parses the output from `bputil --display-all-policies`.
// bputil reference: https://keith.github.io/xcode-man-pages/bputil.1.html
//
// Decriptions of properties: https://support.apple.com/guide/security/contents-a-localpolicy-file-mac-apple-silicon-secc745a0845/web
//
// Example output:
//
// sudo bputil -d
// Password:
//
// This utility is not meant for normal users or even sysadmins.
// It provides unabstracted access to capabilities which are normally handled for the user automatically when changing the security policy through GUIs such as the Startup Security Utility in macOS Recovery ("recoveryOS").
// It is possible to make your system security much weaker and therefore easier to compromise using this tool.
// This tool is not to be used in production environments.
// It is possible to render your system unbootable with this tool.
// It should only be used to understand how the security of Apple Silicon Macs works.
// Use at your own risk!
//
// Local policy for volume group 5D0D176D-E8CC-**REDACTED**:
// OS environment:
// OS Type                                       : macOS
// OS Pairing Status                             : Not Paired
// Local Policy Nonce Hash                 (lpnh): A8D3EC575A03E7F58**REDACTED**
// Remote Policy Nonce Hash                (rpnh): FAD13B6348B0B0FB0**REDACTED**
// Recovery OS Policy Nonce Hash           (ronh): F7FDBA24525C17FFA**REDACTED**
//
// Local policy:
// Pairing Integrity                             : Valid
// Signature Type                                : BAA
// Unique Chip ID                          (ECID): 0x1D68A**REDACTED**
// Board ID                                (BORD): 0x24
// Chip ID                                 (CHIP): 0x8103
// Certificate Epoch                       (CEPO): 0x1
// Security Domain                         (SDOM): 0x1
// Production Status                       (CPRO): 1
// Security Mode                           (CSEC): 1
// OS Version                              (love): 21.1.559.0.0,0
// Volume Group UUID                       (vuid): 8462458F-944E-**REDACTED**
// KEK Group UUID                          (kuid): 41196911-E654-**REDACTED**
// Local Policy Nonce Hash                 (lpnh): A8D3EC575A03E7F58**REDACTED**
// Remote Policy Nonce Hash                (rpnh): FAD13B6348B0B0FB0**REDACTED**
// Next Stage Image4 Hash                  (nsih): 19F3A3DC16816A9FC**REDACTED**
// User Authorized Kext List Hash          (auxp): absent
// Auxiliary Kernel Cache Image4 Hash      (auxi): absent
// Kext Receipt Hash                       (auxr): absent
// CustomKC or fuOS Image4 Hash            (coih): absent
// Security Mode:               Full       (smb0): absent
// 3rd Party Kexts Status:      Disabled   (smb2): absent
// User-allowed MDM Control:    Disabled   (smb3): absent
// DEP-allowed MDM Control:     Disabled   (smb4): absent
// SIP Status:                  Enabled    (sip0): absent
// Signed System Volume Status: Enabled    (sip1): absent
// Kernel CTRR Status:          Enabled    (sip2): absent
// Boot Args Filtering Status:  Enabled    (sip3): absent
func parseStatus(rawdata []byte) ([]map[string]string, error) {
	data := []map[string]string{}

	if len(rawdata) == 0 {
		return nil, errors.New("No data")
	}

	var volumeGroup string

	scanner := bufio.NewScanner(bytes.NewReader(rawdata))
	for scanner.Scan() {
		line := scanner.Text()

		// Always look first for matches with the volume group regexp
		// Once found, it will be the volume_group column associated with subsequent rows
		m := volumeGroupRegexp.FindAllStringSubmatch(line, -1)
		if len(m) == 1 && len(m[0]) == 3 {
			volumeGroup = strings.TrimSpace(m[0][2])
			continue
		}

		// Skip lines not associated with a volume group (includes header warning text)
		if len(volumeGroup) == 0 {
			continue
		}

		// Some lines have one colon, some have two colons
		kv := strings.SplitN(line, ": ", 3)

		var property, mode, code, value string
		switch len(kv) {
		case 2:
			property, code, value = parseTwoColumns(kv)
		case 3:
			property, mode, code, value = parseThreeColumns(kv)
		default:
			// Skip blank lines or other unexpected input
			continue
		}

		rowData := map[string]string{
			"volume_group": volumeGroup,
			"property":     strings.ReplaceAll(strings.TrimSpace(property), " ", "_"),
			"mode":         strings.TrimSpace(mode),
			"code":         code,
			"value":        strings.TrimSpace(value),
		}

		data = append(data, rowData)
	}

	return data, nil
}

// Parses lines which have two columns of data, for example:
//
// Local Policy Nonce Hash                 (lpnh): A8D3EC575A03E7F58**REDACTED**
//
// Signature Type                                : BAA
//
// returns property, code (optional), value
func parseTwoColumns(kv []string) (string, string, string) {
	var property, code, value string
	matches := fourCharCodeRegexp.FindAllStringSubmatch(kv[0], -1)
	if len(matches) > 0 && len(matches[0]) == 4 {
		// matches[0][2] = property name string
		// matches[0][3] = four-character code (4CC) sequence
		property = matches[0][2]
		code = matches[0][3]

	} else {
		property = kv[0]
	}

	value = kv[1]

	return property, code, value
}

// Parses lines which have three columns of data, for example:
//
// 3rd Party Kexts Status:      Disabled   (smb2): absent
//
// returns property, mode, code, value
func parseThreeColumns(kv []string) (string, string, string, string) {
	var property, mode, code, value string
	matches := fourCharCodeRegexp.FindAllStringSubmatch(kv[1], -1)
	if len(matches) > 0 && len(matches[0]) == 4 {
		// matches[0][2] = (Full|Enabled|Disabled)
		// matches[0][3] = four-character code (4CC) sequence
		property = kv[0]
		mode = matches[0][2]
		code = matches[0][3]
		value = kv[2]
	}

	return property, mode, code, value
}
