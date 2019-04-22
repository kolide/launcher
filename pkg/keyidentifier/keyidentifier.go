package keyidentifier

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// keyidentifier attempts to identify a key. It uses a set of
// herusitics to try to guiess what kind, what size, and whether or
// not it's encrypted.

type KeyInfo struct {
	Type       string // Key type. rsa/dsa/etc
	Format     string // file format
	Bits       int    // number of bits in the key
	Encryption string // key encryption algorythem
	Encrypted  bool   // is the key encrypted
	Comment    string // comments attached to the key
	Parser     string // what parser we used to determine information
}

func IdentifyFile(path string) (*KeyInfo, error) {
	keyBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "read key")
	}

	ki, err := Identify(keyBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "reading file %s", path)
	}

	return ki, nil
}

// Identify uses a manually curated set of heuristics to determine
// what kind of key something is.
func Identify(keyBytes []byte) (*KeyInfo, error) {

	// Some magic strings for dispatching
	switch {
	case bytes.HasPrefix(keyBytes, []byte("PuTTY-User-Key-File-2")):
		return decodePuttyPPK(keyBytes)
	case bytes.HasPrefix(keyBytes, []byte("---- BEGIN SSH2")):
		return attemptSshcom(keyBytes)
	case bytes.HasPrefix(keyBytes, []byte("SSH PRIVATE KEY FILE FORMAT 1.1\n")):
		return attemptSsh1(keyBytes)
	}

	// Try various parsers. Note that we consider `err == nil`
	// success. Errors are discarded as an unparsable key

	/*
		// Try the simplest ssh key parsers. These are limited, and don't
		// handle encrypted keys. We may remove them.
		if ki, err := attemptSshParseDSAPrivateKey(keyBytes); err == nil {
			return ki, nil
		}
		if ki, err := attemptSshParseRawPrivateKey(keyBytes); err == nil {
			return ki, nil
		}
	*/

	// Manually parse it from the pem data
	if ki, err := attemptPem(keyBytes); err == nil {
		return ki, nil
	}

	// Out of options
	return nil, errors.New("Unable to parse key")

	/*
		key, err := ssh.ParseRawPrivateKey(keyBytes)
		if err != nil {
			return nil, errors.Wrap(err, "ssh.ParseRawPrivateKey")
		}

		// BitLen won't work -- the length of 0 is 0.
		rsaKey := key.(*rsa.PrivateKey)
		spew.Dump(rsaKey.D.BitLen())
		_ = key
	*/

}

func attemptSsh1(keyBytes []byte) (*KeyInfo, error) {
	if !bytes.HasPrefix(keyBytes, []byte("SSH PRIVATE KEY FILE FORMAT 1.1\n")) {
		return nil, errors.New("key not in ssh1 format")
	}

	ki := &KeyInfo{
		Format: "",
		Parser: "attemptSsh1",
	}

	return ki, nil
}

func attemptSshcom(keyBytes []byte) (*KeyInfo, error) {
	if !bytes.HasPrefix(keyBytes, []byte("---- BEGIN SSH2")) {
		return nil, errors.New("key not in sshcom format")
	}

	ki := &KeyInfo{
		Format: "",
		Parser: "attemptSshcom",
	}

	return ki, nil
}

// attemptPem trie to decode the pem, and then work with the key. It's
// based on code from x/crypto's ssh.ParseRawPrivateKey, but more
// flexible in handling encryption and formats.
func attemptPem(keyBytes []byte) (*KeyInfo, error) {
	ki := &KeyInfo{
		Format: "",
		Parser: "attemptPem",
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("pem could not parse")
	}

	ki.Encrypted = encryptedBlock(block)

	fmt.Printf("pem says: >%s<\n", block.Type)

	switch block.Type {
	case "RSA PRIVATE KEY":
		ki.Type = ssh.KeyAlgoRSA
		ki.Format = "openssh"

		if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			ki.Bits = len(key.PublicKey.N.Bytes()) * 8
		}

		return ki, nil

	case "PRIVATE KEY":
		// RFC5208 - https://tools.ietf.org/html/rfc5208
		ki.Encrypted = x509.IsEncryptedPEMBlock(block)
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			spew.Dump(key)
			switch assertedKey := key.(type) {
			case *rsa.PrivateKey:
				ki.Bits = assertedKey.PublicKey.Size() * 8
				ki.Type = "rsa"
			case *ecdsa.PrivateKey:
				ki.Bits = assertedKey.PublicKey.Curve.Params().BitSize
				ki.Type = "ecdsa"
			}
		}
		return ki, nil

	case "EC PRIVATE KEY":
		if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			ki.Type = "ecdsa"
			ki.Bits = key.PublicKey.Curve.Params().BitSize
		}
		return ki, nil

	case "DSA PRIVATE KEY":
		if key, err := ssh.ParseDSAPrivateKey(block.Bytes); err == nil {
			ki.Bits = len(key.PublicKey.Y.Bytes()) * 8
		}
		ki.Type = ssh.KeyAlgoDSA
		ki.Format = "openssh"
		return ki, nil

	case "OPENSSH PRIVATE KEY":
		ki.Format = "openssh-new"
		// ignore the error.
		parseOpenSSHPrivateKey(ki, block.Bytes)
		return ki, nil

	default:
		// return nil, fmt.Errorf("ssh: unsupported key type %q", block.Type)
		return ki, nil
	}

	return ki, nil
}

func attemptSshParseDSAPrivateKey(keyBytes []byte) (*KeyInfo, error) {
	ki := &KeyInfo{
		Format: "",
		Parser: "ssh.ParseDSAPrivateKey",
	}

	_, err := ssh.ParseDSAPrivateKey(keyBytes)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return ki, nil
}

func attemptSshParseRawPrivateKey(keyBytes []byte) (*KeyInfo, error) {
	ki := &KeyInfo{
		Format: "ssh1",
		Parser: "ssh.ParseRawPrivateKey",
	}

	_, err := ssh.ParseRawPrivateKey(keyBytes)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return ki, nil

}

func decodePuttyPPK(keyBytes []byte) (*KeyInfo, error) {
	ki := &KeyInfo{
		Format: "putty",
		Parser: "decodePuttyPPK",
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
				ki.Encrypted = false
			} else {
				ki.Encrypted = true
				ki.Encryption = components[1]
			}
		case "Comment":
			ki.Comment = components[1]
		}
	}

	return ki, nil
}
