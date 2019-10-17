package keyidentifier

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"
)

const ppkBegin = "PuTTY-User-Key-File-2"

// ParseSshComPrivateKey returns key information from a putty (ppk)
// formatted key file.
func ParsePuttyPrivateKey(keyBytes []byte) (*KeyInfo, error) {
	if !bytes.HasPrefix(keyBytes, []byte(ppkBegin)) {
		return nil, errors.New("missing ppk begin")
	}

	ki := &KeyInfo{
		Format: "putty",
		Parser: "ParsePuttyPrivateKey",
	}

	keyString := string(keyBytes)
	keyString = strings.Replace(keyString, "\r\n", "\n", -1)

	for _, line := range strings.Split(keyString, "\n") {
		components := strings.SplitN(line, ": ", 2)
		if len(components) != 2 {
			continue
		}
		switch components[0] {
		case ppkBegin:
			ki.Type = components[1]
		case "Encryption":
			if components[1] == "none" {
				ki.Encrypted = boolPtr(false)
			} else {
				ki.Encrypted = boolPtr(true)
				ki.Encryption = components[1]
			}
		case "Comment":
			ki.Comment = components[1]
		}
	}

	// this can probably be done much more elegantly
	var publicKeyLines strings.Builder
	inPublicLines := false
	for _, line := range strings.Split(keyString, "\n") {
		if strings.Contains(line, "Public-Lines:") {
			inPublicLines = true
			continue
		}

		if strings.Contains(line, "Private-Lines:") {
			inPublicLines = false
			continue
		}

		if inPublicLines {
			publicKeyLines.WriteString(line)
		}
	}

	md5sum := md5.Sum([]byte(publicKeyLines.String()))

	hexarray := make([]string, len(md5sum))
	for i, c := range md5sum {
		hexarray[i] = hex.EncodeToString([]byte{c})
	}
	ki.Fingerprint = strings.Join(hexarray, ":")

	return ki, nil
}
