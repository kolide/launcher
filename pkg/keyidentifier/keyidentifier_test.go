package keyidentifier

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kolide/kit/logutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestIdentifyFiles walks the testdata directory, and tests each
// file. The expected results are extrapolated from the directory path
func TestIdentifyFiles(t *testing.T) {
	kIdentifer, err := New(WithLogger(logutil.NewCLILogger(true)))
	require.NoError(t, err)

	testFiles := []string{}

	err = filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrap(err, "failure to access path in filepath.Walk")
		}

		if info.IsDir() {
			return nil
		}

		// This automatic tester can only parse things if they're stored
		// in a folder hierarchy. So skip the odd test data
		if strings.HasPrefix(path, "testdata/plaintext") || strings.HasPrefix(path, "testdata/encrypted") {
			testFiles = append(testFiles, path)
			return nil
		}

		return nil
	})
	require.NoError(t, err, "filepath.Walk")

	for _, path := range testFiles {
		testIdentifyFile(t, kIdentifer, path)
	}
}

func testIdentifyFile(t *testing.T, kIdentifer *KeyIdentifier, path string) {
	var err error
	pathComponents := strings.Split(path, "/")
	expected := &KeyInfo{
		Format: pathComponents[4],
	}

	// Key type. It's not wholly clear how we want to identify
	// these. Right now, we do it this way. But it might change.
	switch pathComponents[2] {
	case "rsa":
		expected.Type = "ssh-rsa"
	case "dsa":
		expected.Type = "ssh-dss"
	case "ed25519":
		expected.Type = "ssh-ed25519"
		if expected.Format == "openssh" {
			expected.Format = "openssh-new" // ed25519 is always the new format
		}
	default:
		expected.Type = pathComponents[2]
	}

	if expected.Bits, err = strconv.Atoi(pathComponents[3]); err != nil {
		require.NoError(t, errors.New("can't determine key size"), path)
	}

	switch pathComponents[1] {
	case "encrypted":
		expected.Encrypted = truePtr()
	case "plaintext":
		expected.Encrypted = falsePtr()
	default:
		require.NoError(t, errors.New("can't determine whether this should be encrypted"), path)
	}

	keyInfo, err := kIdentifer.IdentifyFile(path)
	parsedBy := keyInfo.Parser
	keyInfo.Parser = ""
	require.NoError(t, err, path)

	// We're never testing the encryption name
	keyInfo.Encryption = ""

	// The elliptic keys don't always have a clear file format, so don't
	// compare that in this test.
	if expected.Type == "ecdsa" && keyInfo.Format == "" {
		expected.Format = ""
	}

	// The elliptic types carry more detail than we need. So whomp down
	// how we test. eg `ecdsa-sha2-nistp256` becomes `ecdsa` for testing
	if strings.HasPrefix(keyInfo.Type, "ecdsa-") {
		keyInfo.Type = "ecdsa"
	}

	if keyInfo.Format == "openssh" && *keyInfo.Encrypted {
		expected.Bits = 0
	}
	if expected.Type == "ecdsa" && *keyInfo.Encrypted {
		expected.Bits = 0
	}

	// Can't get bit sizes from openssh-new keys. At least not yet.
	if keyInfo.Format == "openssh-new" {
		expected.Bits = 0
	}

	// Can't get bit sizes from putty keys. At least not yet.
	if keyInfo.Format == "putty" {
		expected.Bits = 0
	}

	// Can't get bit size from sshcom key
	if expected.Format == "sshcom" {
		expected.Bits = 0
	}

	// Since Encrypted is a pointer, compare it manually, and then zero it.
	require.Equal(t, expected.Encrypted, keyInfo.Encrypted, "Encypted pointer references")
	expected.Encrypted = nil
	keyInfo.Encrypted = nil

	require.Equal(t, expected, keyInfo, fmt.Sprintf("%s (parsed by %s)", path, parsedBy))

}
