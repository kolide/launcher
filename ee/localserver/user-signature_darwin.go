//go:build darwin
// +build darwin

package localserver

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/vmihailenco/msgpack/v5"
)

func (e *kryptoEcMiddleware) createSecureEnclaveSigner(ctx context.Context, challengeBox challenge.OuterChallenge) (secureEnclaveSigner, error) {
	if e.hardwareSigner == nil || e.hardwareSigner.Public() == nil {
		return nil, fmt.Errorf("no hardware signer")
	}

	// get console user
	uids, err := consoleuser.CurrentUids(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting console user: %w", err)
	}

	if len(uids) == 0 {
		return nil, fmt.Errorf("no console user")
	}

	// should only ever have 1, if we have more than one secure enclave signer will fail
	serverPubKeyB64, err := echelper.PublicEcdsaToB64Der(&e.counterParty)
	if err != nil {
		return nil, fmt.Errorf("converting server public key to b64 der: %w", err)
	}

	challengeBytes, err := challengeBox.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshalling challenge: %w", err)
	}

	ses, err := secureenclavesigner.New(uids[0], serverPubKeyB64, challengeBytes, secureenclavesigner.WithExistingKey(e.hardwareSigner.Public().(*ecdsa.PublicKey)))
	if err != nil {
		return nil, fmt.Errorf("creating secure enclave signer: %w", err)
	}

	return ses, nil
}

func (e *kryptoEcMiddleware) addUserSignature(ctx context.Context, response response, challengeBox challenge.OuterChallenge) (response, error) {
	ses, err := e.createUserSignerFunc(ctx, challengeBox)
	if err != nil {
		return response, fmt.Errorf("creating secure enclave signer: %w", err)
	}

	signResponseOuter, err := ses.Sign(response.Nonce, response.Data)
	if err != nil {
		return response, fmt.Errorf("signing data: %w", err)
	}

	var signResponseInner secureenclavesigner.SignResponseInner
	if err := msgpack.Unmarshal(signResponseOuter.Msg, &signResponseInner); err != nil {
		return response, fmt.Errorf("unmarshalling sign response: %w", err)
	}

	response.Nonce = signResponseInner.Nonce
	response.UserSig = signResponseOuter.Sig

	return response, nil
}
