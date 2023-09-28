package shipper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/launcher"
)

type shipperOption func(*shipper)

// WithUploadURL causes the shipper to upload to the given url instead of requesting a url to upload to
func WithUploadURL(url string) shipperOption {
	return func(s *shipper) {
		s.uploadURL = url
	}
}

// WithNote causes the signed url request to include a human defined note
func WithNote(note string) shipperOption {
	return func(s *shipper) {
		s.note = note
	}
}

type shipper struct {
	writer   *io.PipeWriter
	logger   log.Logger
	knapsack types.Knapsack
	// note is intended to help humans identify the object being shipped
	note string
	// upload url can be set to skip the request for one
	uploadURL string
}

func New(logger log.Logger, knapsack types.Knapsack, opts ...shipperOption) (*shipper, error) {
	s := &shipper{
		logger:   logger,
		knapsack: knapsack,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.uploadURL == "" {
		uploadURL, err := s.signedUrl()
		if err != nil {
			return nil, fmt.Errorf("getting signed url: %w", err)
		}
		s.uploadURL = uploadURL
	}

	reader, writer := io.Pipe()

	go func() {
		uploadResponse, err := http.Post(s.uploadURL, "application/octet-stream", reader)
		if err != nil {
			level.Error(s.logger).Log("msg", "uploading data", "err", err)
		}
		defer uploadResponse.Body.Close()

		uploadRepsonseBody, err := io.ReadAll(uploadResponse.Body)
		if err != nil {
			level.Error(s.logger).Log("msg", "reading upload response", "err", err)
		}

		if uploadResponse.StatusCode != http.StatusOK {
			level.Error(s.logger).Log("msg", "got non 200 status in upload response", "status", uploadResponse.Status, "body", string(uploadRepsonseBody))
		}
	}()

	s.writer = writer
	return s, nil
}

func (s *shipper) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}

func (s *shipper) Close() error {
	return s.writer.Close()
}

func (s *shipper) signedUrl() (string, error) {
	if s.uploadURL != "" {
		return s.uploadURL, nil
	}

	if s.knapsack.DebugUploadRequestURL() == "" {
		return "", errors.New("debug upload request url is empty")
	}

	body, err := launcherData(s.knapsack, s.note)
	if err != nil {
		return "", fmt.Errorf("creating launcher data: %w", err)
	}

	signedUrlRequest, err := http.NewRequest(http.MethodPost, s.knapsack.DebugUploadRequestURL(), bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("creating signed url request: %w", err)
	}

	if err := signHttpRequest(s.knapsack, signedUrlRequest, body); err != nil {
		return "", fmt.Errorf("signing signed url request: %w", err)
	}

	signedUrlResponse, err := http.DefaultClient.Do(signedUrlRequest)
	if err != nil {
		return "", fmt.Errorf("sending signed url request: %w", err)
	}
	defer signedUrlResponse.Body.Close()

	signedUrlResponseBody, err := io.ReadAll(signedUrlResponse.Body)
	if err != nil {
		return "", fmt.Errorf("reading signed url response: %w", err)
	}

	if signedUrlResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("got %s status in signed url response: %s", signedUrlResponse.Status, string(signedUrlResponseBody))
	}

	return string(signedUrlResponseBody), nil
}

func signHttpRequest(k types.Knapsack, req *http.Request, body []byte) error {
	if agent.LocalDbKeys().Public() == nil {
		return nil
	}

	pub, err := echelper.PublicEcdsaToB64Der(agent.LocalDbKeys().Public().(*ecdsa.PublicKey))
	if err != nil {
		return nil
	}

	sig, err := echelper.SignWithTimeout(agent.LocalDbKeys(), body, 1*time.Second, 250*time.Millisecond)
	if err != nil {
		return nil
	}

	req.Header.Set(control.HeaderKey, string(pub))
	req.Header.Set(control.HeaderSignature, base64.StdEncoding.EncodeToString(sig))
	return nil
}

func launcherData(k types.Knapsack, note string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	currentUser := "unknown"
	consoleUsers, err := consoleuser.CurrentUsers(ctx)

	switch {
	case err != nil:
		currentUser = fmt.Sprintf("error getting current users: %s", err)
	case len(consoleUsers) > 0:
		currentUser = consoleUsers[0].Username
	default: // no console users
		currentUser = "no console users"
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("error getting hostname: %s", err)
	}

	b, err := json.Marshal(map[string]string{
		"enroll_secret": enrollSecret(k),
		"username":      currentUser,
		"hostname":      hostname,
		"note":          note,
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
