package control

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/backoff"
)

// HTTPClient handles retrieving control data via HTTP
type HTTPClient struct {
	logger     log.Logger
	addr       string
	baseURL    *url.URL
	client     *http.Client
	insecure   bool
	disableTLS bool
	token      string
}

const (
	HeaderApiVersion = "X-Kolide-Api-Version"
	ApiVersion       = "2023-01-01"
	HeaderChallenge  = "X-Kolide-Challenge"
	HeaderSignature  = "X-Kolide-Signature"
	HeaderKey        = "X-Kolide-Key"
	HeaderSignature2 = "X-Kolide-Signature2"
	HeaderKey2       = "X-Kolide-Key2"
)

type configResponse struct {
	Token  string          `json:"token"`
	Config json.RawMessage `json:"config"`
}

func NewControlHTTPClient(logger log.Logger, addr string, client *http.Client, opts ...HTTPClientOption) (*HTTPClient, error) {
	baseURL, err := url.Parse(fmt.Sprintf("https://%s", addr))
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}
	c := &HTTPClient{
		logger:  logger,
		baseURL: baseURL,
		client:  client,
		addr:    addr,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func (c *HTTPClient) GetConfig() (io.Reader, error) {
	challengeReq, err := http.NewRequest(http.MethodGet, c.url("/api/agent/config").String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create challenge request: %w", err)
	}

	challenge, err := c.do(challengeReq)
	if err != nil {
		return nil, fmt.Errorf("could not make challenge request: %w", err)
	}

	configReq, err := http.NewRequest(http.MethodPost, c.url("/api/agent/config").String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create config request: %w", err)
	}
	configReq.Header.Set(HeaderChallenge, string(challenge))
	configReq.Header.Set("Content-Type", "application/json")
	configReq.Header.Set("Accept", "application/json")

	// Calculate first signature
	localDbKeys := agent.LocalDbKeys()
	if localDbKeys.Type() != "noop" {
		key1, err := keyHeaderValue(localDbKeys)
		if err != nil {
			return nil, fmt.Errorf("could not get key header from local db keys: %w", err)
		}
		sig1, err := signatureHeaderValue(localDbKeys, challenge)
		if err != nil {
			return nil, fmt.Errorf("could not get signature header from local db keys: %w", err)
		}
		configReq.Header.Set(HeaderKey, key1)
		configReq.Header.Set(HeaderSignature, sig1)
	}

	// Calculate second signature if available
	hardwareKeys := agent.HardwareKeys()
	if hardwareKeys.Type() != "noop" {
		key2, err := keyHeaderValue(hardwareKeys)
		if err != nil {
			return nil, fmt.Errorf("could not get key header from hardware keys: %w", err)
		}
		sig2, err := signatureHeaderValue(hardwareKeys, challenge)
		if err != nil {
			return nil, fmt.Errorf("could not get signature header from hardware keys: %w", err)
		}
		configReq.Header.Set(HeaderKey2, key2)
		configReq.Header.Set(HeaderSignature2, sig2)
	}

	configAndAuthKeyRaw, err := c.do(configReq)
	if err != nil {
		return nil, fmt.Errorf("could not make config request: %w", err)
	}

	var cfgResp configResponse
	if err := json.Unmarshal(configAndAuthKeyRaw, &cfgResp); err != nil {
		return nil, fmt.Errorf("could not unmarshal challenge response: %w", err)
	}

	// Set the auth token for use when fetching objects by their hashes later
	c.token = cfgResp.Token

	reader := bytes.NewReader(cfgResp.Config)
	return reader, nil
}

func (c *HTTPClient) GetSubsystemData(hash string) (io.Reader, error) {
	if c.token == "" {
		_, err := c.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("no token set; could not make request to get one: %w", err)
		}
	}

	dataReq, err := http.NewRequest(http.MethodGet, c.url(fmt.Sprintf("/api/agent/object/%s", hash)).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create subsystem data request: %w", err)
	}

	dataReq.Header.Set("Authorization", "Bearer "+c.token)
	dataReq.Header.Set("Content-Type", "application/json")
	dataReq.Header.Set("Accept", "application/json")

	dataRaw, err := c.do(dataReq)
	if err != nil {
		return nil, fmt.Errorf("could not make subsystem data request: %w", err)
	}

	reader := bytes.NewReader(dataRaw)
	return reader, nil
}

func (c *HTTPClient) do(req *http.Request) ([]byte, error) {
	// We always need to include the API version in the headers
	req.Header.Set(HeaderApiVersion, ApiVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non-200 status code %d from control server at %s", resp.StatusCode, resp.Request.URL)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body from control server at %s: %w", resp.Request.URL, err)
	}

	return respBytes, nil
}

func (c *HTTPClient) url(path string) *url.URL {
	u := *c.baseURL
	u.Path = path
	return &u
}

func keyHeaderValue(k crypto.Signer) (string, error) {
	keyDer, err := x509.MarshalPKIXPublicKey(k.Public())
	if err != nil {
		return "", fmt.Errorf("could not marshal public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(keyDer), nil
}

func signatureHeaderValue(k crypto.Signer, challenge []byte) (string, error) {
	var (
		sig []byte
		err error
	)

	// Add a timeout/retry because TPM operations can be slow
	err = backoff.WaitFor(func() error {
		sig, err = echelper.Sign(k, challenge)
		return err
	}, 1*time.Second, 250*time.Millisecond)

	if err != nil {
		return "", fmt.Errorf("could not sign challenge: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}
