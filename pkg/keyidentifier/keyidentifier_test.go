package keyidentifier

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestIdentifyFiles(t *testing.T) {

	testFiles := []string{}

	err := filepath.Walk("testdata/plaintext", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrap(err, "failure to access path in filepath.Walk")
		}

		if path == "testdata/make.sh" {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		testFiles = append(testFiles, path)
		return nil
	})
	require.NoError(t, err, "filepath.Walk")

	// Now that we have paths, let's test them...
	for _, path := range testFiles {
		testIdentifyFile(t, path)
	}
}

func testIdentifyFile(t *testing.T, path string) {
	var err error
	pathComponents := strings.Split(path, "/")
	expected := &KeyInfo{
		Format: pathComponents[4],
	}

	// set  type. Naming is the worst
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
		expected.Encrypted = true
	case "plaintext":
		expected.Encrypted = false
	default:
		require.NoError(t, errors.New("can't determine whether this should be encrypted"), path)
	}

	keyInfo, err := IdentifyFile(path)
	parsedBy := keyInfo.Parser
	keyInfo.Parser = ""
	require.NoError(t, err, path)

	// The elliptic keys don't always have a clear file format, so don't
	// compare that in this test.
	if expected.Type == "ecdsa" && keyInfo.Format == "" {
		expected.Format = ""
	}

	// The elliptic types carry more detail than we need. So whomp down
	// how we test. eg `ecdsa-sha2-nistp256` beco es `ecdsa` for testing
	if strings.HasPrefix(keyInfo.Type, "ecdsa-") {
		keyInfo.Type = "ecdsa"
	}

	// Can't get bit sizes from openssh-new keys. At least not yet.
	if keyInfo.Format == "openssh-new" {
		expected.Bits = 0
	}

	// Can't get bit sizes from putty keys. At least not yet.
	if keyInfo.Format == "putty" {
		expected.Bits = 0
	}

	// FIXME don't test sshcom for now
	if expected.Format == "sshcom" {
		return
	}

	// FIXME skip sshv1
	if expected.Format == "ssh1" {
		return
	}

	require.Equal(t, expected, keyInfo, fmt.Sprintf("%s (parsed by %s)", path, parsedBy))

}
