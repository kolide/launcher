package keyidentifier

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolide/kit/logutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type spec struct {
	KeyPath                   string
	ExpectedFingerprintSHA256 string
	ExpectedFingerprintMD5    string
	Encrypted                 bool
	Bits                      int
	Type                      string
}

// TestIdentifyFiles walks the testdata directory, and tests each
// file.
func TestIdentifyFiles(t *testing.T) {
	kIdentifier, err := New(WithLogger(logutil.NewCLILogger(true)))
	require.NoError(t, err)

	testFiles := []string{}

	err = filepath.Walk("testdata/fingerprints", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrap(err, "failure to access path in filepath.Walk")
		}

		if info.IsDir() {
			return nil
		}

		// all json files in testdata/fingerprints are assumed to be valid test specifications
		if strings.HasSuffix(path, ".json") {
			testFiles = append(testFiles, path)
			return nil
		}

		return nil
	})
	require.NoError(t, err, "filepath.Walk")
	for _, specPath := range testFiles {
		testIdentifyFileV2(t, kIdentifier, specPath)
	}
}

func testIdentifyFileV2(t *testing.T, kIdentifer *KeyIdentifier, specFilePath string) {
	// load the json file
	data, err := ioutil.ReadFile(specFilePath)
	require.NoError(t, err, "reading spec file")
	var example spec
	err = json.Unmarshal(data, &example)
	require.NoError(t, err, "parsing json spec file: %s", specFilePath)
	keyPath := "testdata/fingerprints/" + example.KeyPath

	keyInfo, err := kIdentifer.IdentifyFile(keyPath)
	require.NoError(t, err, "path to unparseable key: %s", keyPath)

	// Key type. It's not wholly clear how we want to identify
	// these. Right now, we do it this way. But it might change.
	switch example.Type {
	case "rsa":
		example.Type = "ssh-rsa"
	case "dsa":
		example.Type = "ssh-dss"
	case "ed25519":
		example.Type = "ssh-ed25519"
		if keyInfo.Format == "openssh" {
			keyInfo.Format = "openssh-new" // ed25519 is always the new format
		}
	default:
	}

	// TODO: need to get encode expected 'format' in the specs
	// The elliptic keys don't always have a clear file format, so don't
	// compare that in this test.
	// if expected.Type == "ecdsa" && keyInfo.Format == "" {
	// 	expected.Format = ""
	// }

	// The elliptic types carry more detail than we need. So whomp down
	// how we test. eg `ecdsa-sha2-nistp256` becomes `ecdsa` for testing
	if strings.HasPrefix(keyInfo.Type, "ecdsa-") {
		example.Type = "ecdsa"
		keyInfo.Type = "ecdsa"
	}

	// Test correct 'bits' reporting
	if keyInfo.Format == "openssh" && *keyInfo.Encrypted {
		// Can't get keys from encrypted openssh keys. At least not yet.
	} else if example.Type == "ecdsa" && *keyInfo.Encrypted {
		// Can't get keys from encrypted ecdsa keys. At least not yet.
	} else if keyInfo.Format == "openssh-new" {
		// Can't get bit sizes from openssh-new keys (yet)
	} else if keyInfo.Format == "putty" {
		// Can't get bit sizes from putty keys. At least not yet.
	} else if keyInfo.Format == "sshcom" {
		// Can't get bit size from sshcom key
	} else {
		require.Equal(t, example.Bits, keyInfo.Bits, "unexpected 'Bits' value")
	}

	// test correct fingerprint reporting. limit support for now
	if keyInfo.Format == "openssh-new" {
		require.Equal(t, example.ExpectedFingerprintSHA256, keyInfo.FingerprintSHA256,
			"unexpected sha256 fingerprint")
		require.Equal(t, example.ExpectedFingerprintMD5, keyInfo.FingerprintMD5,
			"unexpected md5 fingerprint")
	}

	require.Equal(t, example.Type, keyInfo.Type, "unexpected key type")
	if example.Encrypted != *keyInfo.Encrypted {
		// TODO: REMOVE debugging lines
		fmt.Printf("\033[35m%v\033[0m\n", example.KeyPath)
		fmt.Printf("\033[35m%v\033[0m\n", example.Type)
		fmt.Printf("\033[35m%v\033[0m\n", keyInfo.Format)
		// return
	}
	require.Equal(t, example.Encrypted, *keyInfo.Encrypted, "unexpected encrypted boolean")
}
