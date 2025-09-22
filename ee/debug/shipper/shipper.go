package shipper

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
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/ee/control"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/launcher"
)

type shipperOption func(*shipper)

// WithUploadRequestURL causes the shipper to request a url to upload to
func WithUploadRequestURL(url string) shipperOption {
	return func(s *shipper) {
		s.uploadRequestURL = url
	}
}

// WithNote causes the signed url request to include a human defined note
func WithNote(note string) shipperOption {
	return func(s *shipper) {
		s.note = note
	}
}

type shipper struct {
	writer   io.WriteCloser
	knapsack types.Knapsack

	uploadName           string
	uploadRequestURL     string
	uploadRequest        *http.Request
	uploadRequestStarted bool
	uploadRequestErr     error
	uploadResponse       *http.Response
	uploadRequestWg      *sync.WaitGroup
	uploadRequestCancel  context.CancelFunc

	// note is intended to help humans identify the object being shipped
	note string
}

func New(knapsack types.Knapsack, opts ...shipperOption) (*shipper, error) {
	s := &shipper{
		knapsack:        knapsack,
		uploadRequestWg: &sync.WaitGroup{},
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.uploadRequestURL == "" {
		uploadRequestURL, err := url.JoinPath(knapsack.KolideServerURL(), "api/agent/flare")
		if err != nil {
			return nil, fmt.Errorf("joining url: %w", err)
		}
		s.uploadRequestURL = uploadRequestURL
	}

	uploadURL, err := s.signedUrl()
	if err != nil {
		return nil, fmt.Errorf("getting signed url: %w", err)
	}

	reader, writer := io.Pipe()
	s.writer = writer

	req, err := http.NewRequest(http.MethodPut, uploadURL, reader)
	if err != nil {
		return nil, fmt.Errorf("creating request for http upload: %w", err)
	}
	s.uploadRequest = req

	return s, nil
}

func (s *shipper) Name() string {
	return s.uploadName
}

func (s *shipper) Write(p []byte) (n int, err error) {
	if s.uploadRequestStarted {
		return s.writer.Write(p)
	}

	// start request
	// We could start the request in New(), but then we would hold the connection open longer than needed,
	// OTOH, if we started request in New() we would know sooner if we had a bad upload url ... :shrug:
	s.uploadRequestStarted = true
	s.uploadRequestWg.Add(1)
	gowrapper.Go(context.TODO(), s.knapsack.Slogger(), func() {
		defer s.uploadRequestWg.Done()

		// will cancel and close the request body in the close function
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		s.uploadRequestCancel = cancel
		s.uploadResponse, s.uploadRequestErr = http.DefaultClient.Do(s.uploadRequest.WithContext(ctx)) //nolint:bodyclose
	})

	return s.writer.Write(p)
}

func (s *shipper) Close() error {
	if err := s.writer.Close(); err != nil {
		return err
	}

	// this will happen if the write function is never called
	// then nothing sent, no error
	if !s.uploadRequestStarted {
		return nil
	}

	// write has been called -- make sure we call cancel at the end
	defer s.uploadRequestCancel()

	// wait for upload request to finish
	s.uploadRequestWg.Wait()

	if s.uploadRequestErr != nil {
		return fmt.Errorf("upload request error: %w", s.uploadRequestErr)
	}
	defer s.uploadResponse.Body.Close()

	uploadRepsonseBody, err := io.ReadAll(s.uploadResponse.Body)
	if err != nil {
		return fmt.Errorf("reading upload response: %w", err)
	}

	if s.uploadResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("got non 200 status in upload response: %s %s", s.uploadResponse.Status, string(uploadRepsonseBody))
	}

	return nil
}

func (s *shipper) signedUrl() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	body, err := launcherData(s.knapsack, s.note)
	if err != nil {
		return "", fmt.Errorf("creating launcher data: %w", err)
	}

	signedUrlRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, s.uploadRequestURL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("creating signed url request: %w", err)
	}

	signedUrlRequest.Header.Set(control.HeaderApiVersion, control.ApiVersion)
	signedUrlRequest.Header.Set("Content-Type", "application/json")

	signHttpRequest(signedUrlRequest, body)

	signedUrlResponse, err := http.DefaultClient.Do(signedUrlRequest)
	if err != nil {
		return "", fmt.Errorf("sending signed url request: %w", err)
	}
	defer signedUrlResponse.Body.Close()

	responseData := struct {
		URL  string `json:"URL"`
		Name string `json:"name"`
	}{}

	if err := json.NewDecoder(signedUrlResponse.Body).Decode(&responseData); err != nil {
		return "", fmt.Errorf("decoding signed url response: %w", err)
	}

	if signedUrlResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("got %s status in signed url response", signedUrlResponse.Status)
	}

	s.uploadName = responseData.Name
	return responseData.URL, nil
}

func signHttpRequest(req *http.Request, body []byte) {
	sign := func(signer crypto.Signer, headerKey, signatureKey string, request *http.Request) {
		if signer == nil || signer.Public() == nil {
			return
		}

		ecdsaPubKey, ok := signer.Public().(*ecdsa.PublicKey)
		if !ok {
			return
		}
		pub, err := echelper.PublicEcdsaToB64Der(ecdsaPubKey)
		if err != nil {
			return
		}

		sig, err := echelper.SignWithTimeout(signer, body, 1*time.Second, 250*time.Millisecond)
		if err != nil {
			return
		}

		request.Header.Set(headerKey, string(pub))
		request.Header.Set(signatureKey, base64.StdEncoding.EncodeToString(sig))
	}

	sign(agent.LocalDbKeys(), control.HeaderKey, control.HeaderSignature, req)

	// hardware signing is not implemented for darwin
	if runtime.GOOS != "darwin" {
		sign(agent.HardwareKeys(), control.HeaderKey2, control.HeaderSignature2, req)
	}
}

func launcherData(k types.Knapsack, note string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var consoleUsers string
	foundConsoleUsers, err := consoleuser.CurrentUsers(ctx)
	switch {
	case err != nil:
		consoleUsers = fmt.Sprintf("error getting console users: %s", err)
	case len(foundConsoleUsers) == 0:
		consoleUsers = "no console users found"
	default:
		consoleUserNames := make([]string, len(foundConsoleUsers))
		for i, u := range foundConsoleUsers {
			consoleUserNames[i] = u.Username
		}
		consoleUsers = strings.Join(consoleUserNames, ", ")
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("error getting hostname: %s", err)
	}

	runningUser, err := user.Current()
	var runningUsername string
	if err != nil {
		runningUsername = fmt.Sprintf("error getting running user: %s", err)
	} else {
		runningUsername = runningUser.Username
	}

	b, err := json.Marshal(map[string]string{
		"enroll_secret": enrollSecret(k),
		"munemo":        munemo(k),
		"console_users": consoleUsers,
		"running_user":  runningUsername,
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

	if k != nil && k.EnrollSecretPath() != "" {
		secret, err := os.ReadFile(k.EnrollSecretPath())
		if err != nil {
			return ""
		}

		return string(secret)
	}

	// TODO this will need to respect the identifier when determining the secret file location for dual-launcher installations
	// this will specifically be an issue when flare is triggered standalone (without config path specified)
	b, err := os.ReadFile(launcher.DefaultPath(launcher.SecretFile))
	if err != nil {
		return ""
	}

	return string(b)
}

// munemo fetches the registration's munemo from the knapsack. If that is not available,
// it looks for the munemo stored in metadata.json.
func munemo(k types.Knapsack) string {
	if k == nil {
		return munemoFromMetadataJson(launcher.DefaultRootDirectoryPath)
	}

	registrations, err := k.Registrations()
	if err != nil {
		return munemoFromMetadataJson(k.RootDirectory())
	}

	// For now, we can return the munemo for the default registration (also, the only registration currently)
	for _, registration := range registrations {
		if registration.RegistrationID == types.DefaultRegistrationID {
			return registration.Munemo
		}
	}

	return munemoFromMetadataJson(k.RootDirectory())
}

type metadataJson struct {
	OrganizationMunemo string `json:"organization_munemo"`
}

func munemoFromMetadataJson(rootDirectory string) string {
	rawMetadata, err := os.ReadFile(filepath.Join(rootDirectory, "metadata.json"))
	if err != nil {
		return ""
	}
	var m metadataJson
	if err := json.Unmarshal(rawMetadata, &m); err != nil {
		return ""
	}
	return m.OrganizationMunemo
}
