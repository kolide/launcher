//go:build darwin
// +build darwin

package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
	"github.com/kolide/launcher/ee/agent/certs"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/vmihailenco/msgpack/v5"
)

const secureEnclaveTimestampValidityRange = 150

var serverPubKeys = make(map[string]*ecdsa.PublicKey)

// runSecureEnclave performs either a create-key or sign operation using the secure enclave.
// It's available as a separate command because launcher runs a root by default and since it's
// not in a user security context it can't use the secure enclave directly. However, this command
// can be run in the user context using launchctl.
func runSecureEnclave(args []string) error {
	if len(args) < 2 {
		return errors.New("not enough arguments, expect create_key <request> or sign <sign_request>")
	}

	if secureenclavesigner.Undertest {
		if secureenclavesigner.TestServerPubKey == "" {
			return errors.New("test server public key not set")
		}

		k, err := echelper.PublicB64DerToEcdsaKey([]byte(secureenclavesigner.TestServerPubKey))
		if err != nil {
			return fmt.Errorf("parsing test server public key: %w", err)
		}

		serverPubKeys[string(secureenclavesigner.TestServerPubKey)] = k
	}

	for _, keyStr := range []string{certs.K2EccServerCert, certs.ReviewEccServerCert, certs.LocalhostEccServerCert} {
		key, err := echelper.PublicPemToEcdsaKey([]byte(keyStr))
		if err != nil {
			return fmt.Errorf("parsing server public key from pem: %w", err)
		}

		pubB64Der, err := echelper.PublicEcdsaToB64Der(key)
		if err != nil {
			return fmt.Errorf("marshalling server public key to b64 der: %w", err)
		}

		serverPubKeys[string(pubB64Der)] = key
	}

	switch args[0] {
	case secureenclavesigner.CreateKeyCmd:
		return createSecureEnclaveKey(args[1])

	case secureenclavesigner.SignCmd:
		return signWithSecureEnclave(args[1])

	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func createSecureEnclaveKey(requestB64 string) error {
	b, err := base64.StdEncoding.DecodeString(requestB64)
	if err != nil {
		return fmt.Errorf("decoding b64 request: %w", err)
	}

	var request secureenclavesigner.Request
	if err := msgpack.Unmarshal(b, &request); err != nil {
		return fmt.Errorf("unmarshaling msgpack request: %w", err)
	}

	if err := verifySecureEnclaveChallenge(request); err != nil {
		return fmt.Errorf("verifying challenge: %w", err)
	}

	secureEnclavePubKey, err := secureenclave.CreateKey()
	if err != nil {
		return fmt.Errorf("creating secure enclave key: %w", err)
	}

	secureEnclavePubDer, err := echelper.PublicEcdsaToB64Der(secureEnclavePubKey)
	if err != nil {
		return fmt.Errorf("marshalling public key to der: %w", err)
	}

	// write results to stdout since so that parent process can use
	fmt.Println(string(secureEnclavePubDer))
	return nil
}

func signWithSecureEnclave(signRequestB64 string) error {
	b, err := base64.StdEncoding.DecodeString(signRequestB64)
	if err != nil {
		return fmt.Errorf("decoding b64 sign request: %w", err)
	}

	var signRequest secureenclavesigner.SignRequest
	if err := msgpack.Unmarshal(b, &signRequest); err != nil {
		return fmt.Errorf("unmarshaling msgpack sign request: %w", err)
	}

	if err := verifySecureEnclaveChallenge(signRequest.Request); err != nil {
		return fmt.Errorf("verifying challenge: %w", err)
	}

	secureEnclavePubKey, err := echelper.PublicB64DerToEcdsaKey(signRequest.SecureEnclavePubKey)
	if err != nil {
		return fmt.Errorf("marshalling b64 der to public key: %w", err)
	}

	seSigner, err := secureenclave.New(secureEnclavePubKey)
	if err != nil {
		return fmt.Errorf("creating secure enclave signer: %w", err)
	}

	sig, err := seSigner.Sign(rand.Reader, signRequest.Digest, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("signing request: %w", err)
	}

	// write results to stdout since so that parent process can use
	fmt.Print(base64.StdEncoding.EncodeToString(sig))
	return nil
}

func verifySecureEnclaveChallenge(request secureenclavesigner.Request) error {
	c, err := challenge.UnmarshalChallenge(request.Challenge)
	if err != nil {
		return fmt.Errorf("unmarshaling challenge: %w", err)
	}

	serverPubKey, ok := serverPubKeys[string(request.ServerPubKey)]
	if !ok {
		return errors.New("server public key not found")
	}

	if err := c.Verify(*serverPubKey); err != nil {
		return fmt.Errorf("verifying challenge: %w", err)
	}

	// Check the timestamp, this prevents people from saving a challenge and then
	// reusing it a bunch. However, it will fail if the clocks are too far out of sync.
	timestampDelta := time.Now().Unix() - c.Timestamp()
	if timestampDelta > secureEnclaveTimestampValidityRange || timestampDelta < -secureEnclaveTimestampValidityRange {
		return fmt.Errorf("timestamp delta %d is outside of validity range", timestampDelta)
	}

	return nil
}
