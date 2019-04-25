package keyidentifier

import "strings"

func ParsePuttyPrivateKey(keyBytes []byte) (*KeyInfo, error) {
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
		case "PuTTY-User-Key-File-2":
			ki.Type = components[1]
		case "Encryption":
			if components[1] == "none" {
				ki.Encrypted = falsePtr()
			} else {
				ki.Encrypted = truePtr()
				ki.Encryption = components[1]
			}
		case "Comment":
			ki.Comment = components[1]
		}
	}

	return ki, nil

}
