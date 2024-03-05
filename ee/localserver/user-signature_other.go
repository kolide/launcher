//go:build !darwin
// +build !darwin

package localserver

import (
	"context"
	"errors"

	"github.com/kolide/krypto/pkg/challenge"
)

func (e *kryptoEcMiddleware) createSecureEnclaveSigner(ctx context.Context, challengeBox challenge.OuterChallenge) (secureEnclaveSigner, error) {
	return nil, errors.New("not implemented")
}

func (e *kryptoEcMiddleware) addUserSignature(_ context.Context, response response, _ challenge.OuterChallenge) (response, error) {
	return response, errors.New("not implemented")
}
