package keyidentifier

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
	Encrypted  *bool  // is the key encrypted
	Comment    string // comments attached to the key
	Parser     string // what parser we used to determine information
}

type KeyIdentifier struct {
	logger log.Logger
}

type Option func(*KeyIdentifier)

func WithLogger(logger log.Logger) Option {
	return func(kIdentifer *KeyIdentifier) {
		kIdentifer.logger = logger
	}
}

func New(opts ...Option) (*KeyIdentifier, error) {
	kIdentifer := &KeyIdentifier{
		logger: log.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(kIdentifer)
	}

	return kIdentifer, nil
}

func (kIdentifer *KeyIdentifier) IdentifyFile(path string) (*KeyInfo, error) {
	level.Debug(kIdentifer.logger).Log(
		"msg", "starting a key identification",
		"file", path,
	)

	keyBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "read key")
	}

	ki, err := kIdentifer.Identify(keyBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "reading file %s", path)
	}

	return ki, nil
}

// Identify uses a manually curated set of heuristics to determine
// what kind of key something is.
func (kIdentifer *KeyIdentifier) Identify(keyBytes []byte) (*KeyInfo, error) {
	level.Debug(kIdentifer.logger).Log(
		"msg", "starting a key identification",
		"file", "<bytestream>",
	)

	// Some magic strings for dispatching
	switch {
	case bytes.HasPrefix(keyBytes, []byte("PuTTY-User-Key-File-2")):
		return ParsePuttyPrivateKey(keyBytes)
	case bytes.HasPrefix(keyBytes, []byte("---- BEGIN SSH2")):
		return ParseSshComPrivateKey(keyBytes)
	case bytes.HasPrefix(keyBytes, []byte("SSH PRIVATE KEY FILE FORMAT 1.1\n")):
		return ParseSsh1PrivateKey(keyBytes)
	}

	// Try various parsers. Note that we consider `err == nil`
	// success. Errors are discarded as an unparsable key

	// Manually parse it from the pem data
	if ki, err := kIdentifer.attemptPem(keyBytes); err == nil {
		return ki, nil
	}

	// Out of options
	return nil, errors.New("Unable to parse key")

}

// attemptPem tries to decode the pem, and then work with the key. It's
// based on code from x/crypto's ssh.ParseRawPrivateKey, but more
// flexible in handling encryption and formats.
func (kIdentifer *KeyIdentifier) attemptPem(keyBytes []byte) (*KeyInfo, error) {
	ki := &KeyInfo{
		Format: "",
		Parser: "attemptPem",
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("pem could not parse")
	}

	ki.Encrypted = boolPtr(encryptedBlock(block))

	level.Debug(kIdentifer.logger).Log(
		"msg", "pem says",
		"block type", block.Type,
	)

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
		ki.Encrypted = boolPtr(x509.IsEncryptedPEMBlock(block))
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			//spew.Dump(key)
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
		// set the Type here, since parsing fails on encrypted keys
		ki.Type = "ecdsa"

		if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			ki.Type = "ecdsa"
			ki.Bits = key.PublicKey.Curve.Params().BitSize
		} else {
			level.Debug(kIdentifer.logger).Log(
				"msg", "x509.ParseECPrivateKey failed to parse",
				"err", err,
			)
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
	}

	// Unmatched. return what we have
	level.Debug(kIdentifer.logger).Log(
		"msg", "pem failed to match block type",
		"type", block.Type,
	)
	return ki, nil
}

func (kIdentifer *KeyIdentifier) attemptSshParseDSAPrivateKey(keyBytes []byte) (*KeyInfo, error) {
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

func (kIdentifer *KeyIdentifier) attemptSshParseRawPrivateKey(keyBytes []byte) (*KeyInfo, error) {
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
