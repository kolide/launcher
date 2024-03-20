//go:build !darwin
// +build !darwin

package localserver

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/secureenclavesigner"
	"github.com/kolide/launcher/pkg/traces"
)

func (e *kryptoEcMiddleware) challengeResponse(ctx context.Context, o *challenge.OuterChallenge, data []byte) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	response := &secureenclavesigner.SignResponseInner{
		Nonce:     ulid.New(),
		Timestamp: time.Now().UTC().Unix(),
		Data:      []byte(fmt.Sprintf("kolide:%s:kolide", string(data))),
	}

	if e.hardwareSigner == nil || e.hardwareSigner.Public() == nil {
		e.slogger.Log(ctx, slog.LevelError,
			"no hardware signer available",
		)
		traces.SetError(span, errors.New("no hardware signer available"))

		responseBytes, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("marshalling krypto response: %w", err)
		}

		return o.Respond(e.localDbSigner, nil, responseBytes)
	}

	hwSig, err := e.hardwareSigner.Sign(rand.Reader, data, crypto.SHA256)
	if err != nil {
		e.slogger.Log(ctx, slog.LevelError,
			"signing with hardware signer",
			"err", err,
		)
		traces.SetError(span, fmt.Errorf("signing with hardware signer, %w", err))

		responseBytes, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("marshalling krypto response: %w", err)
		}

		return o.Respond(e.localDbSigner, nil, responseBytes)
	}

	response.HardwareSig = hwSig

	response.HardwareKey, err = echelper.PublicEcdsaToB64Der(e.hardwareSigner.Public().(*ecdsa.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("marshalling public signing key to der: %w", err)
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("marshalling krypto response: %w", err)
	}

	return o.Respond(e.localDbSigner, nil, responseBytes)
}
