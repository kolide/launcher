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
	ki.Fingerprint = "<format>" + "<bits>" + strings.Join(hexarray, ":")

	// // taken from https://github.com/golang/crypto/blob/master/ssh/keys.go#L1096
	// sha256sum := sha256.Sum256([]byte(publicKeyLines.String()))
	// hash := base64.RawStdEncoding.EncodeToString(sha256sum[:])
	// ki.Fingerprint = "SHA256:" + hash

	return ki, nil
}
