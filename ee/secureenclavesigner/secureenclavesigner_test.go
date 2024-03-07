//go:build darwin
// +build darwin

package secureenclavesigner

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

const (
	testWrappedEnvVarKey = "SECURE_ENCLAVE_TEST_WRAPPED"
	macOsAppResourceDir  = "./test_app_resources"
)

func WithBinaryPath(p string) opt {
	return func(ses *secureEnclaveSigner) {
		ses.pathToLauncherBinary = p
	}
}

// #nosec G306 -- Need readable files
func TestSecureEnclaveSigner(t *testing.T) {
	t.Parallel()

	if os.Getenv("CI") != "" {
		t.Skipf("\nskipping because %s env var was not empty, this is being run in a CI environment without access to secure enclave", testWrappedEnvVarKey)
	}

	// put the root dir somewhere else if you want to persist the signed macos app bundle
	// should build this into make at some point
	// rootDir := "/tmp/secure_enclave_test"

	rootDir := t.TempDir()
	appRoot := filepath.Join(rootDir, "launcher_test.app")

	// make required dirs krypto_test.app/Contents/MacOS and add files
	require.NoError(t, os.MkdirAll(filepath.Join(appRoot, "Contents", "MacOS"), 0777))
	copyFile(t, filepath.Join(macOsAppResourceDir, "Info.plist"), filepath.Join(appRoot, "Contents", "Info.plist"))
	copyFile(t, filepath.Join(macOsAppResourceDir, "embedded.provisionprofile"), filepath.Join(appRoot, "Contents", "embedded.provisionprofile"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverPrivKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	serverPubKeyDer, err := echelper.PublicEcdsaToB64Der(serverPrivKey.Public().(*ecdsa.PublicKey))
	require.NoError(t, err)

	// build the executable
	executablePath := filepath.Join(appRoot, "Contents", "MacOS", "launcher_test")
	out, err := exec.CommandContext( //nolint:forbidigo // Only used in test, don't want as standard allowedcmd
		ctx,
		"go",
		"build",
		"-ldflags",
		fmt.Sprintf("-X github.com/kolide/launcher/ee/secureenclavesigner.TestServerPubKey=%s", string(serverPubKeyDer)),
		"-tags",
		"secure_enclave_test",
		"-o",
		executablePath,
		"../../cmd/launcher",
	).CombinedOutput()

	require.NoError(t, ctx.Err())
	require.NoError(t, err, string(out))

	// sign app bundle
	signApp(t, appRoot)

	store := inmemory.NewStore()

	// create brand new signer without existing key
	// ask for public first to trigger key generation
	ses, err := New(multislogger.NewNopLogger(), store, WithBinaryPath(executablePath))
	require.NoError(t, err,
		"should be able to create secure enclave signer",
	)

	pubKey := ses.Public()
	require.NotNil(t, pubKey,
		"should be able to create brand new public key",
	)

	pubEcdsaKey := pubKey.(*ecdsa.PublicKey)
	require.NotNil(t, pubEcdsaKey,
		"public key should convert to ecdsa key",
	)

	pubKeySame := ses.Public()
	require.NotNil(t, pubKeySame,
		"should be able to get public key again",
	)

	pubEcdsaKeySame := pubKeySame.(*ecdsa.PublicKey)
	require.NotNil(t, pubEcdsaKeySame,
		"public key should convert to ecdsa key",
	)

	require.Equal(t, pubEcdsaKey, pubEcdsaKeySame,
		"asking for the same public key should return the same key",
	)

	existingDataSes, err := New(multislogger.NewNopLogger(), store, WithBinaryPath(executablePath))
	require.NoError(t, err,
		"should be able to create secure enclave signer with existing key",
	)

	pubKeyUnmarshalled := existingDataSes.Public()
	require.NotNil(t, pubKeyUnmarshalled,
		"should be able to get public key from unmarshalled secure enclave signer",
	)

	pubEcdsaKeyUnmarshalled := pubKeyUnmarshalled.(*ecdsa.PublicKey)
	require.NotNil(t, pubEcdsaKeyUnmarshalled,
		"public key should convert to ecdsa key",
	)

	require.Equal(t, pubEcdsaKey, pubEcdsaKeyUnmarshalled,
		"unmarshalled public key should be the same as original public key",
	)
}

// #nosec G306 -- Need readable files
func copyFile(t *testing.T, source, destination string) {
	bytes, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(destination, bytes, 0700))
}

// #nosec G204 -- This triggers due to using env var in cmd, making exception for test
func signApp(t *testing.T, appRootDir string) {
	codeSignId := os.Getenv("MACOS_CODESIGN_IDENTITY")
	require.NotEmpty(t, codeSignId, "need MACOS_CODESIGN_IDENTITY env var to sign app, such as [Mac Developer: Jane Doe (ABCD123456)]")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext( //nolint:forbidigo // Only used in test, don't want as standard allowcmd
		ctx,
		"codesign",
		"--deep",
		"--force",
		"--options", "runtime",
		"--entitlements", filepath.Join(macOsAppResourceDir, "entitlements"),
		"--sign", codeSignId,
		"--timestamp",
		appRootDir,
	)

	out, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err())
	require.NoError(t, err, string(out))
}
