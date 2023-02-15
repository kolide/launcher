package control

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent"
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

	challengeReader, err := c.do(challengeReq)
	if err != nil {
		return nil, fmt.Errorf("could not make challenge request: %w", err)
	}
	defer challengeReader.Close()

	challenge, err := io.ReadAll(challengeReader)
	if err != nil {
		return nil, fmt.Errorf("could not read challenge from reader: %w", err)
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
	if localDbKeys.Public() == nil {
		return nil, errors.New("cannot request control data without local keys")
	}
	key1, err := echelper.PublicEcdsaToB64Der(localDbKeys.Public().(*ecdsa.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("could not get key header from local db keys: %w", err)
	}
	sig1, err := signatureHeaderValue(localDbKeys, challenge)
	if err != nil {
		return nil, fmt.Errorf("could not get signature header from local db keys: %w", err)
	}
	configReq.Header.Set(HeaderKey, string(key1))
	configReq.Header.Set(HeaderSignature, sig1)

	// Calculate second signature if available
	hardwareKeys := agent.HardwareKeys()
	if hardwareKeys.Public() != nil {
		key2, err := echelper.PublicEcdsaToB64Der(hardwareKeys.Public().(*ecdsa.PublicKey))
		if err != nil {
			return nil, fmt.Errorf("could not get key header from hardware keys: %w", err)
		}
		sig2, err := signatureHeaderValue(hardwareKeys, challenge)
		if err != nil {
			return nil, fmt.Errorf("could not get signature header from hardware keys: %w", err)
		}
		configReq.Header.Set(HeaderKey2, string(key2))
		configReq.Header.Set(HeaderSignature2, sig2)
	}

	configAndAuthKeyRawReader, err := c.do(configReq)
	if err != nil {
		return nil, fmt.Errorf("could not make config request: %w", err)
	}
	defer configAndAuthKeyRawReader.Close()

	configAndAuthKeyRaw, err := io.ReadAll(configAndAuthKeyRawReader)
	if err != nil {
		return nil, fmt.Errorf("could not read config from reader: %w", err)
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

func (c *HTTPClient) GetSubsystemData(hash string) (io.ReadCloser, error) {
	if c.token == "" {
		return nil, errors.New("token is nil, cannot request subsystem data")
	}

	dataReq, err := http.NewRequest(http.MethodGet, c.url(fmt.Sprintf("/api/agent/object/%s", hash)).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create subsystem data request: %w", err)
	}

	dataReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	dataReq.Header.Set("Content-Type", "application/json")
	dataReq.Header.Set("Accept", "application/json")

	reader, err := c.do(dataReq)
	if err != nil {
		return nil, fmt.Errorf("could not make subsystem data request: %w", err)
	}
	return reader, nil
}

// TODO: this should probably just return a io.Reader
func (c *HTTPClient) do(req *http.Request) (io.ReadCloser, error) {
	// We always need to include the API version in the headers
	req.Header.Set(HeaderApiVersion, ApiVersion)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making http request: %w", err)
	}
	//defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non-200 status code %d from control server at %s", resp.StatusCode, resp.Request.URL)
	}

	return resp.Body, nil
}

func (c *HTTPClient) url(path string) *url.URL {
	u := *c.baseURL
	u.Path = path
	return &u
}

func signatureHeaderValue(k crypto.Signer, challenge []byte) (string, error) {
	sig, err := echelper.SignWithTimeout(k, challenge, 1*time.Second, 250*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("could not sign challenge: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}
