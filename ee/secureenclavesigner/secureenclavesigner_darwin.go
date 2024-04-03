//go:build darwin
// +build darwin

package secureenclavesigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/pkg/traces"
)

const (
	CreateKeyCmd     = "create-key"
	SignCmd          = "sign"
	PublicEccDataKey = "publicEccData"
)

type opt func(*secureEnclaveSigner)

type secureEnclaveSigner struct {
	uidPubKeyMap         map[string]*ecdsa.PublicKey
	pathToLauncherBinary string
	store                types.GetterSetterDeleter
	slogger              *slog.Logger
	mux                  *sync.Mutex
}

type SignRequest struct {
	Challenge    []byte `json:"challenge"`
	Data         []byte `json:"data"`
	BaseNonce    string `json:"base_nonce"`
	UserPubkey   []byte `json:"user_pubkey"`
	ServerPubKey []byte `json:"server_pubkey"`
}

func New(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, opts ...opt) (*secureEnclaveSigner, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ses := &secureEnclaveSigner{
		uidPubKeyMap: make(map[string]*ecdsa.PublicKey),
		store:        store,
		slogger:      slogger.With("component", "secureenclavesigner"),
		mux:          &sync.Mutex{},
	}

	data, err := store.Get([]byte(PublicEccDataKey))
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting public ecc data from store: %w", err))
		return nil, fmt.Errorf("getting public ecc data from store: %w", err)
	}

	if data != nil {
		if err := json.Unmarshal(data, ses); err != nil {
			traces.SetError(span, fmt.Errorf("unmarshaling secure enclave signer: %w", err))
			ses.slogger.Log(ctx, slog.LevelError,
				"unable to unmarshal secure enclave signer, data may be corrupt, wiping",
				"err", err,
			)

			if err := store.Delete([]byte(PublicEccDataKey)); err != nil {
				traces.SetError(span, fmt.Errorf("deleting corrupt public ecc data: %w", err))
				return nil, fmt.Errorf("deleting corrupt public ecc data: %w", err)
			}
		}
	}

	for _, opt := range opts {
		opt(ses)
	}

	// this is here to facilitate testing, since go builds a special test binary,
	// if we look for os.Executable in a test and try to exec it, it will error
	if ses.pathToLauncherBinary == "" {
		p, err := os.Executable()
		if err != nil {
			traces.SetError(span, fmt.Errorf("getting path to launcher binary: %w", err))
			return nil, fmt.Errorf("getting path to launcher binary: %w", err)
		}

		ses.pathToLauncherBinary = p
	}

	// get current console user key to make sure it's available
	if _, err := ses.currentConsoleUserKey(ctx); err != nil {
		traces.SetError(span, fmt.Errorf("getting current console user key: %w", err))
		ses.slogger.Log(ctx, slog.LevelError,
			"getting current console user key",
			"err", err,
		)

		// intentionally not returning error here, because this runs on start up
		// and maybe the console user or secure enclave is not available yet
	}

	return ses, nil
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (ses *secureEnclaveSigner) Public() crypto.PublicKey {
	k, err := ses.currentConsoleUserKey(context.TODO())
	if err != nil {
		ses.slogger.Log(context.TODO(), slog.LevelError,
			"getting public key",
			"err", err,
		)
		return nil
	}

	return k
}

func (ses *secureEnclaveSigner) Type() string {
	return "secure_enclave"
}

// Sign is not implemented, it's just here to satisfy the crypto.Signer interface,
// use SignConsoleUser instead
func (ses *secureEnclaveSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (ses *secureEnclaveSigner) SignConsoleUser(ctx context.Context, challenge, data, serverPubkey []byte, baseNonce string) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	signRequest := &SignRequest{
		Challenge:    challenge,
		Data:         data,
		BaseNonce:    baseNonce,
		ServerPubKey: serverPubkey,
	}

	userPubkey, err := ses.currentConsoleUserKey(ctx)
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting current console: %w", err))
		return nil, fmt.Errorf("getting current console user key: %w", err)
	}

	userPubkeyBytes, err := echelper.PublicEcdsaToB64Der(userPubkey)
	if err != nil {
		traces.SetError(span, fmt.Errorf("converting public key to b64 der: %w", err))
		return nil, fmt.Errorf("converting public key to b64 der: %w", err)
	}

	signRequest.UserPubkey = userPubkeyBytes

	signRequestBytes, err := json.Marshal(signRequest)
	if err != nil {
		traces.SetError(span, fmt.Errorf("marshalling sign request: %w", err))
		return nil, fmt.Errorf("marshalling sign request: %w", err)
	}

	signResponseOuterBytes, err := ses.execSign(ctx, base64.StdEncoding.EncodeToString(signRequestBytes))
	if err != nil {
		traces.SetError(span, fmt.Errorf("signing with secure enclave: %w", err))
		return nil, fmt.Errorf("signing with secure enclave: %w", err)
	}

	return signResponseOuterBytes, nil
}

type keyData struct {
	Uid    string `json:"uid"`
	PubKey string `json:"pub_key"`
}

func (ses *secureEnclaveSigner) currentConsoleUserKey(ctx context.Context) (*ecdsa.PublicKey, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ses.mux.Lock()
	defer ses.mux.Unlock()

	cu, err := firstConsoleUser(ctx)
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting first console user: %w", err))
		return nil, fmt.Errorf("getting first console user: %w", err)
	}

	key, ok := ses.uidPubKeyMap[cu.Uid]
	if ok {
		span.AddEvent("found_existing_key_for_console_user")
		return key, nil
	}

	key, err = ses.createKey(ctx, cu)
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating key: %w", err))
		return nil, fmt.Errorf("creating key: %w", err)
	}

	span.AddEvent("created_new_key_for_console_user")

	ses.uidPubKeyMap[cu.Uid] = key
	if err := ses.save(); err != nil {
		delete(ses.uidPubKeyMap, cu.Uid)
		traces.SetError(span, fmt.Errorf("saving secure enclave signer: %w", err))
		return nil, fmt.Errorf("saving secure enclave signer: %w", err)
	}

	span.AddEvent("saved_key_for_console_user")
	return key, nil
}

func (ses *secureEnclaveSigner) MarshalJSON() ([]byte, error) {
	var keyDatas []keyData

	for uid, pubKey := range ses.uidPubKeyMap {
		pubKeyBytes, err := echelper.PublicEcdsaToB64Der(pubKey)
		if err != nil {
			return nil, fmt.Errorf("converting public key to b64 der: %w", err)
		}

		keyDatas = append(keyDatas, keyData{
			Uid:    uid,
			PubKey: string(pubKeyBytes),
		})

	}

	return json.Marshal(keyDatas)
}

func (ses *secureEnclaveSigner) UnmarshalJSON(data []byte) error {
	if ses.uidPubKeyMap == nil {
		ses.uidPubKeyMap = make(map[string]*ecdsa.PublicKey)
	}

	var keyDatas []keyData
	if err := json.Unmarshal(data, &keyDatas); err != nil {
		return fmt.Errorf("unmarshalling key data: %w", err)
	}

	for _, kd := range keyDatas {
		pubKey, err := echelper.PublicB64DerToEcdsaKey([]byte(kd.PubKey))
		if err != nil {
			return fmt.Errorf("converting public key to ecdsa: %w", err)
		}

		ses.uidPubKeyMap[kd.Uid] = pubKey
	}

	return nil
}

func (ses *secureEnclaveSigner) createKey(ctx context.Context, u *user.User) (*ecdsa.PublicKey, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	cmd, err := allowedcmd.Launchctl(
		ctx,
		"asuser",
		u.Uid,
		"sudo",
		"--preserve-env",
		"-u",
		u.Username,
		ses.pathToLauncherBinary,
		"secure-enclave",
		CreateKeyCmd,
	)

	if err != nil {
		traces.SetError(span, fmt.Errorf("creating command to create key: %w", err))
		return nil, fmt.Errorf("creating command to create key: %w", err)
	}

	// skip updates since we have full path of binary
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", "LAUNCHER_SKIP_UPDATES", "true"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		traces.SetError(span, fmt.Errorf("executing launcher binary to create key: %w: %s", err, string(out)))
		return nil, fmt.Errorf("executing launcher binary to create key: %w: %s", err, string(out))
	}

	pubKey, err := echelper.PublicB64DerToEcdsaKey([]byte(lastLine(out)))
	if err != nil {
		traces.SetError(span, fmt.Errorf("converting public key to ecdsa: %w", err))
		return nil, fmt.Errorf("converting public key to ecdsa: %w", err)
	}

	return pubKey, nil
}

func (ses *secureEnclaveSigner) execSign(ctx context.Context, signRequestB64 string) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	u, err := firstConsoleUser(ctx)
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting first console: %w", err))
		return nil, fmt.Errorf("getting first console: %w", err)
	}

	cmd, err := allowedcmd.Launchctl(
		ctx,
		"asuser",
		u.Uid,
		"sudo",
		"--preserve-env",
		"-u",
		u.Username,
		ses.pathToLauncherBinary,
		"secure-enclave",
		SignCmd,
		signRequestB64,
	)

	if err != nil {
		traces.SetError(span, fmt.Errorf("creating sign cmd: %w", err))
		return nil, fmt.Errorf("creating sign cmd:: %w", err)
	}

	// skip updates since we have full path of binary
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", "LAUNCHER_SKIP_UPDATES", "true"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		traces.SetError(span, fmt.Errorf("executing launcher binary to sign with secure enclave: %w: %s", err, string(out)))
		return nil, fmt.Errorf("executing launcher binary to sign with secure enclave: %w: %s", err, string(out))
	}

	return base64.StdEncoding.DecodeString(lastLine(out))
}

// lastLine returns the last line of the out.
// This is needed because laucher sets up a logger by default.
// The last line of the output is the public key or signature.
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

func firstConsoleUser(ctx context.Context) (*user.User, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	c, err := consoleuser.CurrentUsers(ctx)
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting current users: %w", err))
		return nil, fmt.Errorf("getting current users: %w", err)
	}

	if len(c) == 0 {
		traces.SetError(span, errors.New("no console users found"))
		return nil, errors.New("no console users found")
	}

	return c[0], nil
}

func (ses *secureEnclaveSigner) save() error {
	json, err := json.Marshal(ses)
	if err != nil {
		return fmt.Errorf("marshaling secure enclave signer: %w", err)
	}

	if err := ses.store.Set([]byte(PublicEccDataKey), json); err != nil {
		return fmt.Errorf("setting public ecc data: %w", err)
	}

	return nil
}
