//go:build darwin
// +build darwin

package secureenclavesigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/consoleuser"
)

const (
	CreateKeyCmd = "create-key"
)

type opt func(*SecureEnclaveSigner)

type SecureEnclaveSigner struct {
	uidPubKeyMap         map[string]*ecdsa.PublicKey
	pathToLauncherBinary string
	persister            persister
	slogger              *slog.Logger
	mux                  *sync.Mutex
}

// persister is an interface that allows the secure enclave signer to persist
// keys it creates, this is needed since we may need to create a new key anytime
// a new user logs in on macos
type persister interface {
	Persist([]byte) error
}

func New(slogger *slog.Logger, persister persister, opts ...opt) (*SecureEnclaveSigner, error) {
	ses := &SecureEnclaveSigner{
		uidPubKeyMap: make(map[string]*ecdsa.PublicKey),
		persister:    persister,
		slogger:      slogger.With("component", "secureenclavesigner"),
		mux:          &sync.Mutex{},
	}

	for _, opt := range opts {
		opt(ses)
	}

	if ses.pathToLauncherBinary == "" {
		p, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("getting path to launcher binary: %w", err)
		}

		ses.pathToLauncherBinary = p
	}

	return ses, nil
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (ses *SecureEnclaveSigner) Public() crypto.PublicKey {
	ses.mux.Lock()
	defer ses.mux.Unlock()

	c, err := firstConsoleUser()
	if err != nil {
		ses.slogger.Log(context.TODO(), slog.LevelError,
			"getting first console user",
			"err", err,
		)

		return nil
	}

	if v, ok := ses.uidPubKeyMap[c.Uid]; ok {
		return v
	}

	key, err := ses.createKey(context.TODO(), c)
	if err != nil {
		ses.slogger.Log(context.TODO(), slog.LevelError,
			"creating key",
			"err", err,
		)

		return nil
	}

	ses.uidPubKeyMap[c.Uid] = key

	// pesist the new key
	json, err := json.Marshal(ses)
	if err != nil {
		ses.slogger.Log(context.TODO(), slog.LevelError,
			"marshaling secure enclave signer",
			"err", err,
		)

		delete(ses.uidPubKeyMap, c.Uid)
		return nil
	}

	if err := ses.persister.Persist(json); err != nil {
		ses.slogger.Log(context.TODO(), slog.LevelError,
			"persisting secure enclave signer",
			"err", err,
		)
		delete(ses.uidPubKeyMap, c.Uid)
		return nil
	}

	return key
}

func (ses *SecureEnclaveSigner) Type() string {
	return "secure_enclave"
}

func (ses *SecureEnclaveSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

type keyData struct {
	Uid    string `json:"uid"`
	PubKey string `json:"pub_key"`
}

func (ses *SecureEnclaveSigner) MarshalJSON() ([]byte, error) {
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

func (ses *SecureEnclaveSigner) UnmarshalJSON(data []byte) error {
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

func (ses *SecureEnclaveSigner) createKey(ctx context.Context, u *user.User) (*ecdsa.PublicKey, error) {
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
		return nil, fmt.Errorf("creating command to create key: %w", err)
	}

	// skip updates since we have full path of binary
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", "LAUNCHER_SKIP_UPDATES", "true"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("executing launcher binary to create key: %w: %s", err, string(out))
	}

	pubKey, err := echelper.PublicB64DerToEcdsaKey([]byte(lastLine(out)))
	if err != nil {
		return nil, fmt.Errorf("marshalling public key to der: %w", err)
	}

	return pubKey, nil
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

func firstConsoleUser() (*user.User, error) {
	c, err := consoleuser.CurrentUsers(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("getting current users: %w", err)
	}

	if len(c) == 0 {
		return nil, fmt.Errorf("no console users found")
	}

	return c[0], nil
}
