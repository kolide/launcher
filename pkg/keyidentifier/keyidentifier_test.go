package keyidentifier

import (
	"encoding/json"
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
	Format                    string
	Source                    string
}

// TestIdentifyFiles walks the testdata directory, and tests each
// file.
func TestIdentifyFiles(t *testing.T) {
	kIdentifier, err := New(WithLogger(logutil.NewCLILogger(true)))
	require.NoError(t, err)

	testFiles := []string{}

	err = filepath.Walk("testdata/specs", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrap(err, "failure to access path in filepath.Walk")
		}

		if info.IsDir() {
			return nil
		}

		// all json files in testdata/specs are assumed to be valid test specifications
		if strings.HasSuffix(path, ".json") {
			testFiles = append(testFiles, path)
			return nil
		}

		return nil
	})
	require.NoError(t, err, "filepath.Walk")
	for _, specPath := range testFiles {
		testIdentifyFile(t, kIdentifier, specPath)
	}
}

func testIdentifyFile(t *testing.T, kIdentifer *KeyIdentifier, specFilePath string) {
	// load the json file
	data, err := ioutil.ReadFile(specFilePath)
	require.NoError(t, err, "reading spec file")
	var example spec
	err = json.Unmarshal(data, &example)
	require.NoError(t, err, "parsing json spec file: %s", specFilePath)
	keyPath := "testdata/specs/" + example.KeyPath

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
		// ed25519 is always the new format
		if keyInfo.Format == "openssh" || keyInfo.Format == "openssh-new" {
			example.Format = "openssh-new"
			keyInfo.Format = "openssh-new"
		}
	default:
	}

	// The elliptic keys don't always have a clear file format, so don't
	// compare that in this test.
	if example.Type == "ecdsa" && keyInfo.Format == "" {
		example.Format = ""
	}

	// The elliptic types carry more detail than we need. So whomp down
	// how we test. eg `ecdsa-sha2-nistp256` becomes `ecdsa` for testing
	if strings.HasPrefix(keyInfo.Type, "ecdsa-") {
		example.Type = "ecdsa"
		keyInfo.Type = "ecdsa"
	}

	// Test correct 'bits' reporting
	// there are several key types/formats that we don't retrieve 'bits' from yet
	switch {
	case (keyInfo.Format == "openssh" && *keyInfo.Encrypted):
	case (example.Type == "ecdsa" && *keyInfo.Encrypted):
	case (keyInfo.Format == "openssh-new"):
	case (keyInfo.Format == "putty"):
	case (keyInfo.Format == "sshcom"):
	default:
		require.Equal(t, example.Bits, keyInfo.Bits, "unexpected 'Bits' value, path: %s", example.KeyPath)
	}

	// test correct fingerprint reporting. limited support for now
	if keyInfo.Format == "openssh-new" {
		if example.Source != "putty" {
			require.Equal(t, example.ExpectedFingerprintSHA256, keyInfo.FingerprintSHA256,
				"unexpected sha256 fingerprint, path: %s", example.KeyPath)
		}
		require.Equal(t, example.ExpectedFingerprintMD5, keyInfo.FingerprintMD5,
			"unexpected md5 fingerprint, path: %s", example.KeyPath)
	}

	require.Equal(t, example.Format, keyInfo.Format, "unexpected key format, path: %s", example.KeyPath)
	require.Equal(t, example.Type, keyInfo.Type, "unexpected key type, path: %s", example.KeyPath)
	require.Equal(t, example.Encrypted, *keyInfo.Encrypted, "unexpected encrypted boolean, path: %s", example.KeyPath)
}
