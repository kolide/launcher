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
	"os"
	"time"

	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
	"github.com/kolide/launcher/ee/agent/certs"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/vmihailenco/msgpack/v5"
)

const secureEnclaveTimestampValiditySeconds = 150

var serverPubKeys = make(map[string]*ecdsa.PublicKey)

// runSecureEnclave performs either a create-key or sign operation using the secure enclave.
// It's available as a separate command because launcher runs aa root by default and since it's
// not in a user security context, it can't use the secure enclave directly. However, this command
// can be run in the user context using launchctl. To perform an operation, root launcher needs to
// include a challenge signed by a known server. See ee/secureenclavesigner for command data
// structure.
func runSecureEnclave(args []string) error {
	if len(args) < 2 {
		return errors.New("not enough arguments, expect create_key <request> or sign <sign_request>")
	}

	if err := loadServerKeys(); err != nil {
		return fmt.Errorf("loading server keys: %w", err)
	}

	if args[1] == "" {
		return errors.New("missing request")
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

func loadServerKeys() error {
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

	return nil
}

func createSecureEnclaveKey(requestB64 string) error {
	b, err := base64.StdEncoding.DecodeString(requestB64)
	if err != nil {
		return fmt.Errorf("decoding b64 request: %w", err)
	}

	var createKeyRequest secureenclavesigner.CreateKeyRequest
	if err := msgpack.Unmarshal(b, &createKeyRequest); err != nil {
		return fmt.Errorf("unmarshaling msgpack request: %w", err)
	}

	if _, err := extractVerifiedSecureEnclaveChallenge(createKeyRequest.SecureEnclaveRequest); err != nil {
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

	os.Stdout.Write(secureEnclavePubDer)
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

	challenge, err := extractVerifiedSecureEnclaveChallenge(signRequest.SecureEnclaveRequest)
	if err != nil {
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

	digest, err := echelper.HashForSignature(challenge.Msg)
	if err != nil {
		return fmt.Errorf("hashing data for signature: %w", err)
	}

	sig, err := seSigner.Sign(rand.Reader, digest, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("signing request: %w", err)
	}

	os.Stdout.Write([]byte(base64.StdEncoding.EncodeToString(sig)))
	return nil
}

func extractVerifiedSecureEnclaveChallenge(request secureenclavesigner.SecureEnclaveRequest) (*challenge.OuterChallenge, error) {
	challengeUnmarshalled, err := challenge.UnmarshalChallenge(request.Challenge)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling challenge: %w", err)
	}

	serverPubKey, ok := serverPubKeys[string(request.ServerPubKey)]
	if !ok {
		return nil, errors.New("server public key not found")
	}

	if err := challengeUnmarshalled.Verify(*serverPubKey); err != nil {
		return nil, fmt.Errorf("verifying challenge: %w", err)
	}

	// Check the timestamp, this prevents people from saving a challenge and then
	// reusing it a bunch. However, it will fail if the clocks are too far out of sync.
	timestampDelta := time.Now().Unix() - challengeUnmarshalled.Timestamp()
	if timestampDelta > secureEnclaveTimestampValiditySeconds || timestampDelta < -secureEnclaveTimestampValiditySeconds {
		return nil, fmt.Errorf("timestamp delta %d is outside of validity range %d", timestampDelta, secureEnclaveTimestampValiditySeconds)
	}

	return challengeUnmarshalled, nil
}
