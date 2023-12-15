package main

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/vmihailenco/msgpack/v5"
)

func runSecureEnclave(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("not enough arguments, expect create_key <challenge> or sign <sign_request>")
	}

	switch args[0] {
	case "create-key":
		return createSecureEnclaveKey(args[1])

	case "sign":
		return signWithSecureEnclave(args[1])

	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func createSecureEnclaveKey(challenge string) error {
	// TODO: verify challenge

	pubKey, err := secureenclave.CreateKey()
	if err != nil {
		return fmt.Errorf("creating secure enclave key: %w", err)
	}

	pubDer, err := echelper.PublicEcdsaToB64Der(pubKey)
	if err != nil {
		return fmt.Errorf("marshalling public key to der: %w", err)
	}

	fmt.Println(string(pubDer))
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

	// TODO: verify signRequest.Challenge

	pubKey, err := echelper.PublicB64DerToEcdsaKey(signRequest.PubKey)
	if err != nil {
		return fmt.Errorf("marshalling b64 der to public key: %w", err)
	}

	ses, err := secureenclave.New(pubKey)
	if err != nil {
		return fmt.Errorf("creating secure enclave signer: %w", err)
	}
	var sig []byte
	backoff.WaitFor(func() error {
		sig, err = ses.Sign(rand.Reader, signRequest.Digest, crypto.SHA256)
		return err
	}, 250*time.Millisecond, 2*time.Second)

	if err != nil {
		return fmt.Errorf("signing request: %w", err)
	}

	fmt.Print(base64.StdEncoding.EncodeToString(sig))
	return nil
}
