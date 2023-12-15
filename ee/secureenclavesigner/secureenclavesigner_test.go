package secureenclavesigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/echelper"
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

// TestSecureEnclaveTestRunner TODO: description
// #nosec G306 -- Need readable files
func TestSecureEnclaveSigner(t *testing.T) {
	t.Parallel()

	if os.Getenv("CI") != "" {
		t.Skipf("\nskipping because %s env var was not empty, this is being run in a CI environment without access to secure enclave", testWrappedEnvVarKey)
	}

	// set up app bundle
	rootDir := t.TempDir()
	appRoot := filepath.Join(rootDir, "launcher_test.app")

	// make required dirs krypto_test.app/Contents/MacOS and add files
	require.NoError(t, os.MkdirAll(filepath.Join(appRoot, "Contents", "MacOS"), 0777))
	copyFile(t, filepath.Join(macOsAppResourceDir, "Info.plist"), filepath.Join(appRoot, "Contents", "Info.plist"))
	copyFile(t, filepath.Join(macOsAppResourceDir, "embedded.provisionprofile"), filepath.Join(appRoot, "Contents", "embedded.provisionprofile"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// build the executable
	executablePath := filepath.Join(appRoot, "Contents", "MacOS", "launcher_test")
	out, err := exec.CommandContext(ctx, "go", "build", "-o", executablePath, "../../cmd/launcher").CombinedOutput()
	require.NoError(t, ctx.Err())
	require.NoError(t, err, string(out))

	// sign app bundle
	signApp(t, appRoot)

	usr, err := user.Current()
	require.NoError(t, err)

	// create brand new signer without existing key
	// ask for public first to trigger key generation
	ses, err := New(usr.Uid, []byte("TODO:challenge"), WithBinaryPath(executablePath))
	require.NoError(t, err)

	pubKey := ses.Public()
	require.NotNil(t, pubKey)

	dataToSign := []byte(ulid.New())
	digest, err := echelper.HashForSignature(dataToSign)
	require.NoError(t, err)

	sigB64, err := ses.Sign(rand.Reader, digest, crypto.SHA256)
	require.NoError(t, err)

	sig, err := base64.StdEncoding.DecodeString(string(sigB64))
	require.NoError(t, err)

	require.NoError(t, echelper.VerifySignature(pubKey.(*ecdsa.PublicKey), dataToSign, sig))

	// create brand new signer without existing key
	// ask to sign first to trigger key generation
	ses, err = New(usr.Uid, []byte("TODO:challenge"), WithBinaryPath(executablePath))
	require.NoError(t, err)

	sigB64, err = ses.Sign(rand.Reader, digest, crypto.SHA256)
	require.NoError(t, err)

	sig, err = base64.StdEncoding.DecodeString(string(sigB64))
	require.NoError(t, err)

	require.NoError(t, echelper.VerifySignature(ses.Public().(*ecdsa.PublicKey), dataToSign, sig))

	// create signer with existing key
	ses, err = New(usr.Uid, []byte("TODO:challenge"), WithBinaryPath(executablePath), WithExistingKey(pubKey.(*ecdsa.PublicKey)))
	require.NoError(t, err)

	sigB64, err = ses.Sign(rand.Reader, digest, crypto.SHA256)
	require.NoError(t, err)

	sig, err = base64.StdEncoding.DecodeString(string(sigB64))
	require.NoError(t, err)

	require.NoError(t, echelper.VerifySignature(pubKey.(*ecdsa.PublicKey), dataToSign, sig))

	pubKey = ses.Public()
	require.NotNil(t, pubKey)
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

	cmd := exec.CommandContext(
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
