package shipping

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"github.com/kolide/launcher/pkg/launcher"
)

func Ship(logger log.Logger, k types.Knapsack, dataToShip io.Reader) error {
	// first get a signed url
	if k.DebugUploadRequestURL() == "" {
		return errors.New("debug upload request url is empty")
	}

	launcherData, err := launcherData(k)
	if err != nil {
		return fmt.Errorf("creating launcher data: %w", err)
	}

	signedUrlRequest, err := http.NewRequest(http.MethodPost, k.DebugUploadRequestURL(), launcherData)
	if err != nil {
		return fmt.Errorf("creating signed url request: %w", err)
	}

	if err := signHttpRequest(k, signedUrlRequest); err != nil {
		return fmt.Errorf("signing signed url request: %w", err)
	}

	signedUrlResponse, err := http.DefaultClient.Do(signedUrlRequest)
	if err != nil {
		return fmt.Errorf("sending signed url request: %w", err)
	}
	defer signedUrlResponse.Body.Close()

	signedUrlResponseBody, err := io.ReadAll(signedUrlResponse.Body)
	if err != nil {
		return fmt.Errorf("reading signed url response: %w", err)
	}

	if signedUrlResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("got %s status in signed url response: %s", signedUrlResponse.Status, string(signedUrlResponseBody))
	}

	// now upload to the signed url
	uploadResponse, err := http.Post(string(signedUrlResponseBody), "application/octet-stream", dataToShip)
	if err != nil {
		return fmt.Errorf("uploading data: %w", err)
	}
	defer uploadResponse.Body.Close()

	uploadRepsonseBody, err := io.ReadAll(uploadResponse.Body)
	if err != nil {
		return fmt.Errorf("reading upload response: %w", err)
	}

	if uploadResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("got %s status in upload response: %s", uploadResponse.Status, string(uploadRepsonseBody))
	}

	return nil
}

func signHttpRequest(k types.Knapsack, req *http.Request) error {
	if agent.LocalDbKeys().Public() == nil {
		return nil
	}

	pub, err := echelper.PublicEcdsaToB64Der(agent.LocalDbKeys().Public().(*ecdsa.PublicKey))
	if err != nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("reading body to create signature: %w", err)
	}

	req.Body.Close()

	// put the body back
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	sig, err := echelper.SignWithTimeout(agent.LocalDbKeys(), bodyBytes, 1*time.Second, 250*time.Millisecond)
	if err != nil {
		return nil
	}

	req.Header.Set(control.HeaderKey, string(pub))
	req.Header.Set(control.HeaderSignature, base64.StdEncoding.EncodeToString(sig))
	return nil
}

func launcherData(k types.Knapsack) (io.Reader, error) {
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

	return bytes.NewBuffer(b), nil
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
