//go:build darwin
// +build darwin

package secureenclavesigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/vmihailenco/msgpack/v5"
)

type SignRequest struct {
	Request
	// Digest is the hash []byte of the data to be signed
	Digest []byte `msgpack:"digest"`
	// SecureEnclavePubKey is the B64 encoded DER of the public key to be used to verify the signature
	SecureEnclavePubKey []byte `msgpack:"secure_enclave_pub_key"`
}

type Request struct {
	// Challenge is the B64 encoded challenged generated by the server
	Challenge []byte `msgpack:"challenge"`
	// ServerPubKey is the B64 encoded DER of the public key
	// to be used to verify the signature of the request
	ServerPubKey []byte `msgpack:"server_pub_key"`
}

type opt func(*secureEnclaveSigner)

// WithExistingKey allows you to pass the public portion of an existing
// secure enclave key to use for signing
func WithExistingKey(publicKey *ecdsa.PublicKey) opt {
	return func(ses *secureEnclaveSigner) {
		ses.pubKey = publicKey
	}
}

type secureEnclaveSigner struct {
	uid                  string
	serverPubKeyB64Der   []byte
	challenge            []byte
	pubKey               *ecdsa.PublicKey
	pathToLauncherBinary string
}

func New(uid string, serverPubKeyB64Der []byte, challenge []byte, opts ...opt) (*secureEnclaveSigner, error) {
	ses := &secureEnclaveSigner{
		uid:                uid,
		serverPubKeyB64Der: serverPubKeyB64Der,
		challenge:          challenge,
	}

	for _, opt := range opts {
		opt(ses)
	}

	if ses.pathToLauncherBinary == "" {
		p, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("getting path to launcher binary: %w", err)
		}

		ses.pathToLauncherBinary = p
	}

	return ses, nil
}

// Public returns the public key of the secure enclave signer
// it creates a new public key using secure enclave if a public key
// is not set
func (ses *secureEnclaveSigner) Public() crypto.PublicKey {
	if ses.pubKey != nil {
		return ses.pubKey
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := ses.createKey(ctx); err != nil {
		return nil
	}

	return ses.pubKey
}

// Sign signs the digest using the secure enclave
// If a public key is not set, it will create a new key
func (ses *secureEnclaveSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	// create the key if we don't have it
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if ses.pubKey == nil {
		if err := ses.createKey(ctx); err != nil {
			return nil, fmt.Errorf("creating key: %w", err)
		}
	}

	pubKeyBytes, err := echelper.PublicEcdsaToB64Der(ses.pubKey)
	if err != nil {
		return nil, fmt.Errorf("marshalling public key to der: %w", err)
	}

	signRequest := SignRequest{
		Request: Request{
			Challenge:    ses.challenge,
			ServerPubKey: ses.serverPubKeyB64Der,
		},
		Digest:              digest,
		SecureEnclavePubKey: pubKeyBytes,
	}

	signRequestMsgPack, err := msgpack.Marshal(signRequest)
	if err != nil {
		return nil, fmt.Errorf("marshalling sign request to msgpack: %w", err)
	}

	// get user name from uid
	u, err := user.LookupId(ses.uid)
	if err != nil {
		return nil, fmt.Errorf("looking up user by uid: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"/bin/launchctl",
		"asuser",
		ses.uid,
		"sudo",
		"--preserve-env",
		"-u",
		u.Username,
		ses.pathToLauncherBinary,
		"secure-enclave",
		"sign",
		base64.StdEncoding.EncodeToString(signRequestMsgPack),
	)

	// skip updates since we have full path of binary
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", "LAUNCHER_SKIP_UPDATES", "true"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("executing launcher binary to sign: %w: %s", err, string(out))
	}

	return []byte(lastLine(out)), nil
}

func (ses *secureEnclaveSigner) createKey(ctx context.Context) error {
	// get user name from uid
	u, err := user.LookupId(ses.uid)
	if err != nil {
		return fmt.Errorf("looking up user by uid: %w", err)
	}

	request := Request{
		Challenge:    ses.challenge,
		ServerPubKey: ses.serverPubKeyB64Der,
	}

	requestMsgPack, err := msgpack.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshalling request to msgpack: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"/bin/launchctl",
		"asuser",
		ses.uid,
		"sudo",
		"--preserve-env",
		"-u",
		u.Username,
		ses.pathToLauncherBinary,
		"secure-enclave",
		"create-key",
		base64.StdEncoding.EncodeToString(requestMsgPack),
	)

	// skip updates since we have full path of binary
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", "LAUNCHER_SKIP_UPDATES", "true"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("executing launcher binary to create key: %w: %s", err, string(out))
	}

	pubKey, err := echelper.PublicB64DerToEcdsaKey([]byte(lastLine(out)))
	if err != nil {
		return fmt.Errorf("marshalling public key to der: %w", err)
	}

	ses.pubKey = pubKey
	return nil
}

func lastLine(out []byte) string {
	outStr := string(out)

	// get last line of outstr
	lastLine := ""
	for _, line := range strings.Split(outStr, "\n") {
		if line != "" {
			lastLine = line
		}
	}

	return lastLine
}
