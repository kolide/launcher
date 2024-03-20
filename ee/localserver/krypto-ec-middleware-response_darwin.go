//go:build darwin
// +build darwin

package localserver

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
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

	inner := secureenclavesigner.SignResponseInner{
		Nonce:     ulid.New(),
		Timestamp: time.Now().UTC().Unix(),
		Data:      data,
	}

	if e.hardwareSigner == nil || e.hardwareSigner.Public() == nil {
		e.slogger.Log(ctx, slog.LevelInfo,
			"no hardware signer available",
		)
		traces.SetError(span, errors.New("no hardware signer available"))

		// add the kolide:%s:kolide tags since the secure enclave cmd
		// wont be execed and add them
		inner.Data = []byte(fmt.Sprintf("kolide:%s:kolide", string(inner.Data)))
		innerBytes, err := json.Marshal(inner)
		if err != nil {
			return nil, fmt.Errorf("marshalling krypto middleware inner response: %w", err)
		}

		outerBytes, err := json.Marshal(kryptoEcMiddlewareResponse{
			Msg: base64.StdEncoding.EncodeToString(innerBytes),
		})

		if err != nil {
			return nil, fmt.Errorf("marshalling krypto middleware outer response: %w", err)
		}

		return o.Respond(e.localDbSigner, nil, outerBytes)
	}

	challengeBytes, err := o.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshalling challenge: %w", err)
	}

	serverKeyBytes, err := echelper.PublicEcdsaToB64Der(&e.counterParty)
	if err != nil {
		return nil, fmt.Errorf("converting counter party public key to bytes: %w", err)
	}

	signResponseOuterBytes, err := e.hardwareSigner.SignConsoleUser(ctx, challengeBytes, data, serverKeyBytes, inner.Nonce)
	if err != nil {
		e.slogger.Log(ctx, slog.LevelError,
			"signing with console user hardware key",
			"err", err,
		)
		traces.SetError(span, fmt.Errorf("signing with console user hardware key, %w", err))

		// add the kolide:%s:kolide tags since the secure enclave cmd failed
		// and did not add them
		inner.Data = []byte(fmt.Sprintf("kolide:%s:kolide", string(inner.Data)))
		innerBytes, err := json.Marshal(inner)
		if err != nil {
			return nil, fmt.Errorf("marshalling krypto middleware inner response: %w", err)
		}

		outerBytes, err := json.Marshal(kryptoEcMiddlewareResponse{
			Msg: base64.StdEncoding.EncodeToString(innerBytes),
		})

		if err != nil {
			return nil, fmt.Errorf("marshalling krypto middleware outer response: %w", err)
		}

		return o.Respond(e.localDbSigner, nil, outerBytes)
	}

	hardwareKeyBytes, err := echelper.PublicEcdsaToB64Der(e.hardwareSigner.Public().(*ecdsa.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("marshalling hardware public signing key to der: %w", err)
	}

	var signResponseOuter secureenclavesigner.SignResponseOuter
	if err := json.Unmarshal(signResponseOuterBytes, &signResponseOuter); err != nil {
		return nil, fmt.Errorf("unmarshaling sign response outer: %w", err)
	}

	var signResponseInner secureenclavesigner.SignResponseInner
	if err := json.Unmarshal(signResponseOuter.Msg, &signResponseInner); err != nil {
		return nil, fmt.Errorf("unmarshaling sign response inner: %w", err)
	}

	// the secure enclave cmd will add the kolide:%s:kolide tags
	inner.Data = signResponseInner.Data
	// use nonce from the secure enclave cmd
	inner.Nonce = signResponseInner.Nonce
	// use timestamp from the secure enclave cmd
	inner.Timestamp = signResponseInner.Timestamp

	innerBytes, err := json.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("marshalling krypto middleware inner response: %w", err)
	}

	outer := kryptoEcMiddlewareResponse{
		Msg:         base64.StdEncoding.EncodeToString(innerBytes),
		HardwareSig: base64.StdEncoding.EncodeToString(signResponseOuter.Sig),
		HardwareKey: string(hardwareKeyBytes),
	}

	outerBytes, err := json.Marshal(outer)
	if err != nil {
		return nil, fmt.Errorf("marshalling krypto middleware outer response: %w", err)
	}

	return o.Respond(e.localDbSigner, nil, outerBytes)
}
