package keyidentifier

// These functions are copied from
// https://github.com/golang/crypto/blob/master/ssh/keys.go They've
// been modified to do what I need

import (
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

type dsaPublicKey struct {
}

// encryptedBlock tells whether a private key is
// encrypted by examining its Proc-Type header
// for a mention of ENCRYPTED
// according to RFC 1421 Section 4.6.1.1.
func encryptedBlock(block *pem.Block) bool {
	return strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED")
}

// Implemented based on the documentation at
// https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.key
func parseOpenSSHPrivateKey(ki *KeyInfo, key []byte) error {
	const magic = "openssh-key-v1\x00"
	if len(key) < len(magic) || string(key[:len(magic)]) != magic {
		return errors.New("ssh: invalid openssh private key format")
	}
	remaining := key[len(magic):]

	var w struct {
		CipherName   string
		KdfName      string
		KdfOpts      string
		NumKeys      uint32
		PubKey       []byte
		PrivKeyBlock []byte
	}

	if err := ssh.Unmarshal(remaining, &w); err != nil {
		return err
	}

	if w.KdfName != "none" || w.CipherName != "none" {
		ki.Encrypted = true
		ki.Encryption = fmt.Sprintf("%s-%s", w.CipherName, w.KdfName)
	} else {
		ki.Encrypted = false
	}

	// If we can parse the public key. extract info
	if pubKey, err := ssh.ParsePublicKey(w.PubKey); err == nil {
		ki.Type = pubKey.Type()
		// We ought be able to get the size of the key, but I don't see
		// how it's exposed. The ssh.PublicKey type is very bare.
		// ki.Bits = len(pubKey.Parameters().Y.Bytes()) * 8
	}

	// That's what we can get from non-encrypted.
	return nil
}
