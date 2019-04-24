package keyidentifier

import (
	"bytes"
	"encoding/binary"
	"encoding/pem"
	"io"
	"strings"

	"github.com/pkg/errors"
)

// Cobbled together from various sources:
//
// Putty
// https://github.com/poderosaproject/poderosa/blob/da6a0512d510fc5f02f065a33727f4bbff19a8cb/Granados/Poderosa/KeyFormat/SSHComPrivateKeyLoader.cs

const sshcomBegin = "---- BEGIN SSH2 ENCRYPTED PRIVATE KEY ----"
const sshcomMagicNumber = 1064303083 // This is 0x3f6ff9eb

//   var pemStart = "-----BEGIN "

// ParseSshComPrivateKey returns key information from an ssh.com key.
func ParseSshComPrivateKey(keyBytes []byte) (*KeyInfo, error) {

	if !bytes.HasPrefix(keyBytes, []byte(sshcomBegin)) {
		return nil, errors.New("missing sshcom header")
	}

	// ssh2 looks like a pem, but uses a different start and end block
	// designation. So we re-write them to look like a pem block and
	// then hand it to pem to decode
	keyBytes = bytes.Replace(keyBytes, []byte("---- BEGIN"), []byte("-----BEGIN"), 1)
	keyBytes = bytes.Replace(keyBytes, []byte("---- END"), []byte("-----END"), 1)
	keyBytes = bytes.Replace(keyBytes, []byte("KEY ----"), []byte("KEY-----"), -1)

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, errors.New("pem could not parse block")
	}

	//if !bytes.HasPrefix(block.Bytes, sshcomMagicNumber) {
	//	return nil, errors.New("missing magic number")
	//}

	var sshData struct {
		Magic     uint32
		KeyLength uint32
	}

	blockReader := bytes.NewReader(block.Bytes)

	// TODO: Is this ever Little Endian!?
	if err := binary.Read(blockReader, binary.BigEndian, &sshData); err != nil {
		return nil, errors.Wrap(err, "binary read")
	}

	if sshData.Magic != sshcomMagicNumber {
		return nil, errors.New("missing magic number")
	}

	keyType, err := readString(blockReader)
	if err != nil {
		return nil, errors.Wrap(err, "readstring keyType")
	}

	cipherName, err := readString(blockReader)
	if err != nil {
		return nil, errors.Wrap(err, "cipherName")
	}

	ki := &KeyInfo{
		Format: "sshcom",
		Parser: "ParseSshComPrivateKey",
	}

	switch cipherName {
	case "none":
		ki.Encrypted = falsePtr()
	case "3des-cbc":
		ki.Encrypted = truePtr()
		ki.Encryption = cipherName
	default:
		return nil, errors.Errorf("sshcom bad cipher name: %s. Should be none or 3des-cbc", cipherName)
	}

	switch {
	case strings.HasPrefix(keyType, "if-modn{sign{rsa"):
		ki.Type = "ssh-rsa"
	case strings.HasPrefix(keyType, "dl-modp{sign{dsa"):
		ki.Type = "ssh-dss" //FIXME
	default:
		return nil, errors.Errorf("Unknown key type: %s", keyType)
	}

	return ki, nil

}

func readString(r *bytes.Reader) (string, error) {
	strLenBytes := make([]uint8, 4)
	r.Read(strLenBytes)

	strLen := binary.BigEndian.Uint32(strLenBytes)

	if strLen > uint32(r.Len()) {
		return "", errors.Errorf("requsted %d, but only %d remain", strLen, r.Len())
	}

	str := make([]byte, strLen)
	if _, err := io.ReadFull(r, str); err != nil {
		return "", errors.Wrap(err, "error reading buffer")
	}

	return string(str), nil
}
