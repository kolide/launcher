//go:build !darwin
// +build !darwin

package tpm

import (
	"crypto"
	"crypto/rsa"
	"fmt"

	"github.com/google/go-tpm-tools/client"
	protoTpm "github.com/google/go-tpm-tools/proto/tpm"
	"github.com/google/go-tpm/tpm2"
	"google.golang.org/protobuf/encoding/prototext"
)

const (
	CryptoHash   = crypto.SHA256
	MaxSealBytes = 128
	signingAlgo  = tpm2.AlgSHA256
)

func signingKeyTemplate() tpm2.Public {
	keyTemplate := client.AKTemplateRSA()
	keyTemplate.RSAParameters.Sign.Hash = signingAlgo
	return keyTemplate
}

// Seal seals the provided input with the TPMs storage root key.
// Sealing includes encryption plus additional data to verify the state of the TPM.
func Seal(input []byte) ([]byte, error) {
	if len(input) > MaxSealBytes {
		return nil, fmt.Errorf("input (length %d) exceeded max input length %d", len(input), MaxSealBytes)
	}

	rwc, err := tpm2.OpenTPM()
	if err != nil {
		return nil, fmt.Errorf("opening TPM: %w", err)
	}
	defer rwc.Close()

	storageRootKey, err := client.StorageRootKeyRSA(rwc)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	defer storageRootKey.Close()

	opts := client.SealOpts{
		Current: tpm2.PCRSelection{
			Hash: signingAlgo,
		},
	}

	sealed, err := storageRootKey.Seal(input, opts)
	if err != nil {
		return nil, fmt.Errorf("sealing data: %w", err)
	}

	marshallOpts := prototext.MarshalOptions{
		Multiline: true,
		EmitASCII: true,
	}

	output, err := marshallOpts.Marshal(sealed)
	if err != nil {
		return nil, fmt.Errorf("marshaling sealed data: %w", err)
	}

	return output, nil
}

// Unseal unseals the provided input with the TPMs storage root key.
// Unsealing includes decryption plus additional verificaiton of the state of the TPM.
func Unseal(input []byte) ([]byte, error) {
	rwc, err := tpm2.OpenTPM()
	if err != nil {
		return nil, fmt.Errorf("opening TPM: %w", err)
	}
	defer rwc.Close()

	var sealed protoTpm.SealedBytes
	unmarshalOptions := prototext.UnmarshalOptions{}
	if err := unmarshalOptions.Unmarshal(input, &sealed); err != nil {
		return nil, fmt.Errorf("unmarshaling sealed data: %w", err)
	}

	storageRootKey, err := client.StorageRootKeyRSA(rwc)
	if err != nil {
		return nil, fmt.Errorf("getting key key: %w", err)
	}
	defer storageRootKey.Close()

	opts := client.UnsealOpts{
		CertifyCurrent: tpm2.PCRSelection{
			Hash: client.CertifyHashAlgTpm,
		},
	}

	output, err := storageRootKey.Unseal(&sealed, opts)
	if err != nil {
		return nil, fmt.Errorf("unsealing data: %w", err)
	}

	return output, nil
}

// Sign signs the provided input with a generated key.
// The keys are derived deterministically from the TPM built-in and protected seed.
// This means the keys will always be the same as long as the TPM is not reset.
// Use PublicSigningKey() to get the public key
func Sign(input []byte) ([]byte, error) {
	rwc, err := tpm2.OpenTPM()
	if err != nil {
		return nil, fmt.Errorf("opening TPM: %w", err)
	}
	defer rwc.Close()

	signingKey, err := client.NewKey(rwc, tpm2.HandleEndorsement, signingKeyTemplate())
	if err != nil {
		return nil, fmt.Errorf("opening signing key: %w", err)
	}
	defer signingKey.Close()

	hash := CryptoHash.New()
	hash.Write(input)

	signedData, err := signingKey.SignData(input)
	if err != nil {
		return nil, fmt.Errorf("signing data: %w", err)
	}

	if err := rsa.VerifyPKCS1v15(signingKey.PublicKey().(*rsa.PublicKey), CryptoHash, hash.Sum(nil), signedData); err != nil {
		return nil, fmt.Errorf("signed data verification failed: %w", err)
	}

	return signedData, nil
}

// PublicSigningKey returns the public key of the key used for signing.
// The key is derived deterministically from the TPM built-in and protected seed.
// This means the keys will always be the same as long as the TPM is not reset.
func PublicSigningKey() (*rsa.PublicKey, error) {
	rwc, err := tpm2.OpenTPM()
	if err != nil {
		return nil, fmt.Errorf("opening TPM: %w", err)
	}
	defer rwc.Close()

	signingKey, err := client.NewKey(rwc, tpm2.HandleEndorsement, signingKeyTemplate())
	if err != nil {
		return nil, fmt.Errorf("opening signing key: %w", err)
	}
	defer signingKey.Close()

	return signingKey.PublicKey().(*rsa.PublicKey), nil
}
