package service

import (
	"encoding/hex"
	"strings"
)

const utf8Replacement = "�"

func redactNonUTF8(in string) string {
	return strings.ToValidUTF8(in, utf8Replacement)
}

// patchOsqueryEmojiHandling repairs utf8 data in the logs. See:
//
// https://github.com/kolide/launcher/issues/445
// https://github.com/facebook/osquery/issues/5288
func patchOsqueryEmojiHandling(in string) string {
	if !strings.Contains(in, `\x`) {
		return in
	}

	out := strings.Replace(in, `\x`, ``, -1)
	outBytes, err := hex.DecodeString(out)
	if err != nil {
		return in
	}

	return string(outBytes)
}

// patchOsqueryEmojiHandlingArray calls patchOsqueryEmojiHandling across an array
func patchOsqueryEmojiHandlingArray(logs []string) []string {
	out := make([]string, len(logs))

	for i, in := range logs {
		out[i] = patchOsqueryEmojiHandling(in)
	}

	return out
}
