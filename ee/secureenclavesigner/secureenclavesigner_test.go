//go:build darwin
// +build darwin

package secureenclavesigner

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	testWrappedEnvVarKey                  = "SECURE_ENCLAVE_TEST_WRAPPED"
	testSkipSecureEnclaveTestingEnvVarKey = "SKIP_SECURE_ENCLAVE_TESTS"
	macOsAppResourceDir                   = "./test_app_resources"
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

	if os.Getenv(testSkipSecureEnclaveTestingEnvVarKey) != "" {
		t.Skipf("\nskipping because %s env var was set", testSkipSecureEnclaveTestingEnvVarKey)
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
	ses, err := New(context.TODO(), multislogger.NewNopLogger(), store, WithBinaryPath(executablePath))
	require.NoError(t, err,
		"should be able to create secure enclave signer",
	)

	userPubKey := ses.Public()
	require.NotNil(t, userPubKey,
		"should be able to create brand new public key",
	)

	userPubEcdsaKey := userPubKey.(*ecdsa.PublicKey)
	require.NotNil(t, userPubEcdsaKey,
		"public key should convert to ecdsa key",
	)

	userPubKeySame := ses.Public()
	require.NotNil(t, userPubKeySame,
		"should be able to get public key again",
	)

	userPubEcdsaKeySame := userPubKeySame.(*ecdsa.PublicKey)
	require.NotNil(t, userPubEcdsaKeySame,
		"public key should convert to ecdsa key",
	)

	require.True(t, userPubEcdsaKey.Equal(userPubEcdsaKeySame),
		"asking for the same public key should return the same key",
	)

	existingDataSes, err := New(context.TODO(), multislogger.NewNopLogger(), store, WithBinaryPath(executablePath))
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

	require.Equal(t, userPubEcdsaKey, pubEcdsaKeyUnmarshalled,
		"unmarshalled public key should be the same as original public key",
	)

	challengeBytes, _, err := challenge.Generate(serverPrivKey, []byte(ulid.New()), []byte(ulid.New()), []byte(ulid.New()))
	require.NoError(t, err,
		"should be able to generate challenge",
	)

	baseNonce := ulid.New()
	signResponseOuterBytes, err := ses.SignConsoleUser(ctx, challengeBytes, []byte(ulid.New()), serverPubKeyDer, baseNonce)
	require.NoError(t, err,
		"should be able to sign with secure enclave",
	)

	var signResponseOuter SignResponseOuter
	require.NoError(t, json.Unmarshal(signResponseOuterBytes, &signResponseOuter),
		"should be able to unmarshal sign response outer",
	)

	require.NoError(t, echelper.VerifySignature(userPubEcdsaKey, signResponseOuter.Msg, signResponseOuter.Sig),
		"should be able to verify signature",
	)

	var signResponseInner SignResponseInner
	require.NoError(t, json.Unmarshal(signResponseOuter.Msg, &signResponseInner),
		"should be able to unmarshal sign response inner",
	)

	require.True(t, strings.HasPrefix(string(signResponseInner.Data), "kolide:"),
		"signed data should have prefix",
	)

	require.True(t, strings.HasSuffix(string(signResponseInner.Data), ":kolide"),
		"signed data should have suffix",
	)

	require.True(t, strings.HasPrefix(signResponseInner.Nonce, baseNonce),
		"nonce should have base nonce prefix",
	)

	require.InDelta(t, time.Now().UTC().Unix(), signResponseInner.Timestamp, 5,
		"timestamp should be within 5 seconds of now",
	)

	// test some error cases
	_, err = ses.SignConsoleUser(ctx, nil, []byte(ulid.New()), serverPubKeyDer, ulid.New())
	require.Error(t, err,
		"should not be able to sign with nil challenge",
	)

	_, err = ses.SignConsoleUser(ctx, challengeBytes, []byte(ulid.New()), nil, ulid.New())
	require.Error(t, err,
		"should not be able to sign with nil serverkey",
	)

	randomKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err,
		"should be able to generate ecdsa key",
	)

	randomKeyMarshalled, err := echelper.PublicEcdsaToB64Der(randomKey.Public().(*ecdsa.PublicKey))
	require.NoError(t, err,
		"should be able to marshal ecdsa key",
	)

	_, err = ses.SignConsoleUser(ctx, challengeBytes, []byte(ulid.New()), randomKeyMarshalled, ulid.New())
	require.Error(t, err,
		"should not be able to sign with unrecognized serverkey",
	)

	challengeBoxOuter, err := challenge.UnmarshalChallenge(challengeBytes)
	require.NoError(t, err,
		"should be able to unmarshal challenge",
	)

	var challengeBoxInner *challenge.InnerChallenge
	require.NoError(t, msgpack.Unmarshal(challengeBoxOuter.Msg, &challengeBoxInner),
		"should be able to unmarshal inner challenge",
	)

	challengeBoxInner.Timestamp = time.Now().Unix() - 1000000
	challengeBoxOuter.Msg, err = msgpack.Marshal(challengeBoxInner)
	require.NoError(t, err,
		"should be able to marshal inner challenge",
	)

	challengeBytes, err = challengeBoxOuter.Marshal()
	require.NoError(t, err,
		"should be able to marshal challenge",
	)

	_, err = ses.SignConsoleUser(ctx, challengeBytes, []byte(ulid.New()), serverPubKeyDer, ulid.New())
	require.ErrorContains(t, err, "invalid signature",
		"any tampering should invalidate signature",
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
