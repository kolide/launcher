//go:build darwin
// +build darwin

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	testWrappedEnvVarKey = "SECURE_ENCLAVE_TEST_WRAPPED"
	macOsAppResourceDir  = "../../ee/secureenclavesigner/test_app_resources"
)

// TestSecureEnclaveTestRunner creates a MacOS app with the binary of this packages tests, then signs the app with entitlements and runs the tests.
// This is done because in order to access secure enclave to run tests, we need MacOS entitlements.
// #nosec G306 -- Need readable files
func TestSecureEnclaveTestRunner(t *testing.T) {
	t.Parallel()

	if os.Getenv("CI") != "" {
		t.Skipf("\nskipping because %s env var was not empty, this is being run in a CI environment without access to secure enclave", testWrappedEnvVarKey)
	}

	if os.Getenv(testWrappedEnvVarKey) != "" {
		t.Skipf("\nskipping because %s env var was not empty, this is the execution of the codesigned app with entitlements", testWrappedEnvVarKey)
	}

	t.Log("\nexecuting wrapped tests with codesigned app and entitlements")

	// set up app bundle
	rootDir := t.TempDir()
	appRoot := filepath.Join(rootDir, "launcher_test.app")

	// make required dirs krypto_test.app/Contents/MacOS and add files
	require.NoError(t, os.MkdirAll(filepath.Join(appRoot, "Contents", "MacOS"), 0700))
	copyFile(t, filepath.Join(macOsAppResourceDir, "Info.plist"), filepath.Join(appRoot, "Contents", "Info.plist"))
	copyFile(t, filepath.Join(macOsAppResourceDir, "embedded.provisionprofile"), filepath.Join(appRoot, "Contents", "embedded.provisionprofile"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// build an executable containing the tests into the app bundle
	executablePath := filepath.Join(appRoot, "Contents", "MacOS", "launcher_test")
	out, err := exec.CommandContext(ctx, "go", "test", "-c", "--cover", "--race", "./", "-o", executablePath).CombinedOutput()
	require.NoError(t, ctx.Err())
	require.NoError(t, err, string(out))

	// sign app bundle
	signApp(t, appRoot)

	// run app bundle executable
	cmd := exec.CommandContext(ctx, executablePath, "-test.v")
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", testWrappedEnvVarKey, "true"))
	out, err = cmd.CombinedOutput()
	require.NoError(t, ctx.Err())
	require.NoError(t, err, string(out))

	// ensure the test ran
	require.Contains(t, string(out), "PASS: TestSecureEnclaveCmd")
	require.NotContains(t, string(out), "FAIL")
	t.Log(string(out))
}

func TestSecureEnclaveCmd(t *testing.T) {
	t.Parallel()

	if os.Getenv(testWrappedEnvVarKey) == "" {
		t.Skipf("\nskipping because %s env var was empty, test not being run from codesigned app with entitlements", testWrappedEnvVarKey)
	}

	t.Log("\nrunning wrapped tests with codesigned app and entitlements")

	oldStdout := os.Stdout
	defer func() {
		os.Stdout = oldStdout
	}()

	// create a pipe to capture stdout
	pipeReader, pipeWriter, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = pipeWriter

	require.NoError(t, runSecureEnclave([]string{"create-key", "TODO:challenge"}))
	require.NoError(t, pipeWriter.Close())

	var buf bytes.Buffer
	_, err = buf.ReadFrom(pipeReader)
	require.NoError(t, err)

	createKeyResponse := buf.Bytes()

	pubKey, err := echelper.PublicB64DerToEcdsaKey(createKeyResponse)
	require.NoError(t, err)
	require.NotNil(t, pubKey, "should be able to get public key")

	dataToSign := []byte(ulid.New())
	digest, err := echelper.HashForSignature(dataToSign)
	require.NoError(t, err)

	signRequest := secureenclavesigner.SignRequest{
		Challenge: []byte("TODO:challenge"),
		Digest:    digest,
		PubKey:    createKeyResponse,
	}

	signRequestMsgPack, err := msgpack.Marshal(signRequest)
	require.NoError(t, err)

	signRequestB64 := base64.StdEncoding.EncodeToString(signRequestMsgPack)

	pipeReader, pipeWriter, err = os.Pipe()
	require.NoError(t, err)

	os.Stdout = pipeWriter
	require.NoError(t, runSecureEnclave([]string{"sign", signRequestB64}))
	require.NoError(t, pipeWriter.Close())

	buf = bytes.Buffer{}
	_, err = buf.ReadFrom(pipeReader)
	require.NoError(t, err)

	sig, err := base64.StdEncoding.DecodeString(string(buf.Bytes()))
	require.NoError(t, err)

	require.NoError(t, echelper.VerifySignature(pubKey, dataToSign, sig))
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
