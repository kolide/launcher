//go:build darwin
// +build darwin

package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
	"github.com/kolide/launcher/ee/agent/certs"
	"github.com/kolide/launcher/ee/secureenclavesigner"
)

const secureEnclaveTimestampValiditySeconds = 10

// runSecureEnclave performs either a create-key operation using the secure enclave.
// It's available as a separate command because launcher runs as root by default and since it's
// not in a user security context, it can't use the secure enclave directly. However, this command
// can be run in the user context using launchctl.
func runSecureEnclave(args []string) error {
	// currently we are just creating key, but plan to add sign command in future
	if len(args) < 1 {
		return errors.New("not enough arguments, expect create_key | sign")
	}

	switch args[0] {
	case secureenclavesigner.CreateKeyCmd:
		return createSecureEnclaveKey()
	case secureenclavesigner.SignCmd:
		if len(args) < 2 {
			return errors.New("not enough arguments for sign command, expect sign <data>")
		}

		return signWithSecureEnclave(args[1])
	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func createSecureEnclaveKey() error {
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
	if err := json.Unmarshal(b, &signRequest); err != nil {
		return fmt.Errorf("unmarshaling msgpack sign request: %w", err)
	}

	if err := verifySecureEnclaveChallenge(signRequest); err != nil {
		return fmt.Errorf("verifying challenge: %w", err)
	}

	userPubKey, err := echelper.PublicB64DerToEcdsaKey(signRequest.UserPubkey)
	if err != nil {
		return fmt.Errorf("marshalling b64 der to public key: %w", err)
	}

	seSigner, err := secureenclave.New(userPubKey)
	if err != nil {
		return fmt.Errorf("creating secure enclave cmd signer: %w", err)
	}

	// tag the ends of the data to sign, this is intended to ensure that launcher wont
	// sign arbitrary things, any party verifying the signature will need to
	// handle these tags
	dataToSign := []byte(fmt.Sprintf("kolide:%s:kolide", signRequest.Data))

	signResponseInner := secureenclavesigner.SignResponseInner{
		Nonce:     fmt.Sprintf("%s%s", signRequest.BaseNonce, ulid.New()),
		Timestamp: time.Now().UTC().Unix(),
		Data:      dataToSign,
	}

	innerResponseBytes, err := json.Marshal(signResponseInner)
	if err != nil {
		return fmt.Errorf("marshalling inner response: %w", err)
	}

	digest, err := echelper.HashForSignature(innerResponseBytes)
	if err != nil {
		return fmt.Errorf("hashing data for signature: %w", err)
	}

	sig, err := seSigner.Sign(rand.Reader, digest, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("signing request: %w", err)
	}

	outerResponseBytes, err := json.Marshal(secureenclavesigner.SignResponseOuter{
		Msg: innerResponseBytes,
		Sig: sig,
	})

	if err != nil {
		return fmt.Errorf("marshalling outer response: %w", err)
	}

	os.Stdout.Write([]byte(base64.StdEncoding.EncodeToString(outerResponseBytes)))
	return nil
}

func verifySecureEnclaveChallenge(signRequest secureenclavesigner.SignRequest) error {
	challengeUnmarshalled, err := challenge.UnmarshalChallenge(signRequest.Challenge)
	if err != nil {
		return fmt.Errorf("unmarshaling challenge: %w", err)
	}

	serverPubKey, err := loadSecureEnclaveServerPubKey(string(signRequest.ServerPubKey))
	if err != nil {
		return fmt.Errorf("loading server public key: %w", err)
	}

	if err := challengeUnmarshalled.Verify(*serverPubKey); err != nil {
		return fmt.Errorf("verifying challenge: %w", err)
	}

	// Check the timestamp, this prevents people from saving a challenge and then
	// reusing it a bunch. However, it will fail if the clocks are too far out of sync.
	timestampDelta := time.Now().Unix() - challengeUnmarshalled.Timestamp()
	if timestampDelta > secureEnclaveTimestampValiditySeconds || timestampDelta < -secureEnclaveTimestampValiditySeconds {
		return fmt.Errorf("timestamp delta %d is outside of validity range %d", timestampDelta, secureEnclaveTimestampValiditySeconds)
	}

	return nil
}

func loadSecureEnclaveServerPubKey(b64Key string) (*ecdsa.PublicKey, error) {
	providedKey, err := echelper.PublicB64DerToEcdsaKey([]byte(b64Key))
	if err != nil {
		return nil, fmt.Errorf("parsing provided server public key: %w", err)
	}

	if secureenclavesigner.Undertest {
		if secureenclavesigner.TestServerPubKey == "" {
			return nil, errors.New("test server public key not set")
		}

		k, err := echelper.PublicB64DerToEcdsaKey([]byte(secureenclavesigner.TestServerPubKey))
		if err != nil {
			return nil, fmt.Errorf("parsing test server public key: %w", err)
		}

		if !providedKey.Equal(k) {
			return nil, errors.New("provided server public key does not match test server public key")
		}

		return k, nil
	}

	for _, serverKey := range []string{certs.K2EccServerCert, certs.ReviewEccServerCert, certs.LocalhostEccServerCert} {
		k, err := echelper.PublicPemToEcdsaKey([]byte(serverKey))
		if err != nil {
			continue
		}

		if providedKey.Equal(k) {
			return k, nil
		}
	}

	return nil, errors.New("provided server public key does not match any known server public key")
}
