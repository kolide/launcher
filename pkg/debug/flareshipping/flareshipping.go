package flareshipping

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/launcher"
)

type flarer interface {
	RunFlare(ctx context.Context, k types.Knapsack, flareStream io.Writer, runtimeEnvironment checkups.RuntimeEnvironmentType) error
}

func RunFlareShip(logger log.Logger, k types.Knapsack, flarer flarer, requestUrl string) error {
	// make sure we have a url to upload to
	uploadUrl, err := requestUploadUrl(k, requestUrl)
	if err != nil {
		return fmt.Errorf("requesting upload url: %w", err)
	}

	flareBuf := bytes.NewBuffer([]byte{})

	// run flare
	ctx := context.Background()
	if err := flarer.RunFlare(ctx, k, flareBuf, checkups.InSituEnvironment); err != nil {
		return fmt.Errorf("running flare: %w", err)
	}

	flareUploadRequest, err := http.NewRequest(http.MethodPost, uploadUrl, flareBuf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	flareUploadResponse, err := http.DefaultClient.Do(flareUploadRequest)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer flareUploadResponse.Body.Close()

	flareUploadResponseBody, err := io.ReadAll(flareUploadResponse.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if len(flareUploadResponseBody) > 0 {
		logger.Log("flare upload response", string(flareUploadResponseBody))
	}

	if flareUploadResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("got %s status in response: %s", flareUploadResponse.Status, string(flareUploadResponseBody))
	}

	return nil
}

func requestUploadUrl(k types.Knapsack, requestUrl string) (string, error) {
	data, err := uploadUrlRequestData(k)
	if err != nil {
		return "", fmt.Errorf("creating request body: %w", err)
	}

	request, err := uploadUrlRequest(k, requestUrl, data)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("got %s status in response: %s", response.Status, string(body))
	}

	return string(body), nil
}

func uploadUrlRequest(k types.Knapsack, url string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// try to get access to launcher keys so we can sign request, but if we fail at any point
	// just return the request without the signature and let server side deal with it
	launcherKey := launcherKey(k)
	if launcherKey == nil {
		return req, nil
	}

	pub, err := echelper.PublicEcdsaToB64Der(launcherKey.Public().(*ecdsa.PublicKey))
	if err != nil {
		return req, nil
	}

	sig, err := echelper.SignWithTimeout(launcherKey, body, 1*time.Second, 250*time.Millisecond)
	if err != nil {
		return req, nil
	}

	req.Header.Set(control.HeaderKey, string(pub))
	req.Header.Set(control.HeaderSignature, base64.StdEncoding.EncodeToString(sig))
	return req, nil
}

func uploadUrlRequestData(k types.Knapsack) ([]byte, error) {
	user, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting username: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("getting hostname: %w", err)
	}

	b, err := json.Marshal(map[string]string{
		"enroll_secret": enrollSecret(k),
		"username":      user.Username,
		"hostname":      hostname,
	})

	if err != nil {
		return nil, fmt.Errorf("marshaling data: %w", err)
	}

	return b, nil
}

func enrollSecret(k types.Knapsack) string {
	// we may be running as launcher daemon or we may be calling this directly in an
	// independent process that is not set up with knapsack
	if k != nil && k.EnrollSecret() != "" {
		return k.EnrollSecret()
	}

	b, err := os.ReadFile(launcher.DefaultPath(launcher.SecretFile))
	if err != nil {
		return ""
	}

	return string(b)
}

func launcherKey(k types.Knapsack) crypto.Signer {
	if k == nil || k.ConfigStore() == nil {
		return nil
	}

	if err := agent.SetupKeys(log.NewNopLogger(), k.ConfigStore()); err != nil {
		return nil
	}

	// should we let flare create db and add keys?
	return agent.LocalDbKeys()
}
