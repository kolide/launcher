package control

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/observability"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPClient handles retrieving control data via HTTP
type HTTPClient struct {
	addr       string
	baseURL    *url.URL
	client     *http.Client
	insecure   bool
	disableTLS bool
	token      string
	slogger    *slog.Logger
}

const (
	HeaderApiVersion = "X-Kolide-Api-Version"
	ApiVersion       = "2023-01-01"
	HeaderChallenge  = "X-Kolide-Challenge"
	HeaderSignature  = "X-Kolide-Signature"
	HeaderKey        = "X-Kolide-Key"
	HeaderSignature2 = "X-Kolide-Signature2"
	HeaderKey2       = "X-Kolide-Key2"

	defaultRequestTimeout = 30 * time.Second
)

type configResponse struct {
	Token  string          `json:"token"`
	Config json.RawMessage `json:"config"`
}

func NewControlHTTPClient(addr string, client *http.Client, logger *slog.Logger, opts ...HTTPClientOption) (*HTTPClient, error) {
	baseURL, err := url.Parse(fmt.Sprintf("https://%s", addr))
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}
	c := &HTTPClient{
		baseURL: baseURL,
		client:  client,
		addr:    addr,
		slogger: logger,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.client.Transport = otelhttp.NewTransport(c.client.Transport)

	return c, nil
}

func (c *HTTPClient) GetConfig(ctx context.Context) (io.Reader, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	challengeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/agent/config").String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create challenge request: %w", err)
	}

	challenge, err := c.do(challengeReq)
	if err != nil {
		return nil, fmt.Errorf("could not make challenge request: %w", err)
	}

	configReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/agent/config").String(), nil)
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
	ecdsaPubKey, ok := localDbKeys.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("local db keys in unexpected format (expected ECDSA, got %T)", localDbKeys.Public())
	}
	key1, err := echelper.PublicEcdsaToB64Der(ecdsaPubKey)
	if err != nil {
		return nil, fmt.Errorf("could not get key header from local db keys: %w", err)
	}
	sig1, err := signatureHeaderValue(localDbKeys, challenge)
	if err != nil {
		return nil, fmt.Errorf("could not get signature header from local db keys: %w", err)
	}
	configReq.Header.Set(HeaderKey, string(key1))
	configReq.Header.Set(HeaderSignature, sig1)

	if err := c.setHardwareKeyHeader(configReq, challenge); err != nil {
		c.slogger.Log(ctx, slog.LevelWarn,
			"failed to set hardware key header, not fatal moving on",
			"err", err,
		)
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

func (c *HTTPClient) setHardwareKeyHeader(req *http.Request, challenge []byte) error {
	if runtime.GOOS == "darwin" {
		// Hardware key signing not supported on darwin
		return nil
	}

	hardwareKeys := agent.HardwareKeys()

	if agent.HardwareKeys() == nil || hardwareKeys.Public() == nil {
		return errors.New("nil hardware keys")
	}

	ecdsaPubKey, ok := hardwareKeys.Public().(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("hardware keys in unexpected format (expected ECDSA, got %T)", hardwareKeys.Public())
	}
	key2, err := echelper.PublicEcdsaToB64Der(ecdsaPubKey)
	if err != nil {
		return fmt.Errorf("could not get key header from hardware keys: %w", err)
	}

	sig2, err := signatureHeaderValue(hardwareKeys, challenge)
	if err != nil {
		return fmt.Errorf("could not get signature header from hardware keys: %w", err)
	}

	req.Header.Set(HeaderKey2, string(key2))
	req.Header.Set(HeaderSignature2, sig2)
	return nil
}

func (c *HTTPClient) GetSubsystemData(ctx context.Context, hash string) (io.Reader, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	if c.token == "" {
		return nil, errors.New("token is nil, cannot request subsystem data")
	}

	dataReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(fmt.Sprintf("/api/agent/object/%s", hash)).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not create subsystem data request: %w", err)
	}

	dataReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	dataReq.Header.Set("Content-Type", "application/json")
	dataReq.Header.Set("Accept", "application/json")

	dataRaw, err := c.do(dataReq)
	if err != nil {
		return nil, fmt.Errorf("could not make subsystem data request: %w", err)
	}

	reader := bytes.NewReader(dataRaw)
	return reader, nil
}

// SendMessage sends a message to the server using JSON-RPC format
func (c *HTTPClient) SendMessage(ctx context.Context, method string, params interface{}) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	if c.token == "" {
		return errors.New("token is nil, cannot send message to server")
	}

	bodyMap := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}

	if params == nil {
		delete(bodyMap, "params")
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return fmt.Errorf("could not marshal message body: %w", err)
	}

	const maxMessageSize = 1024
	if len(body) > maxMessageSize {
		return fmt.Errorf("message size %d exceeds maximum size %d", len(body), maxMessageSize)
	}

	dataReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/agent/message").String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("could not create server message: %w", err)
	}

	dataReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	dataReq.Header.Set("Content-Type", "application/json")
	dataReq.Header.Set("Accept", "application/json")

	// we don't care about the response here, just want to know
	// if there was an error sending our request
	_, err = c.do(dataReq)
	return err
}

// TODO: this should probably just return a io.Reader
func (c *HTTPClient) do(req *http.Request) ([]byte, error) {
	req, span := observability.StartHttpRequestSpan(req)
	defer span.End()

	// Ensure we set a timeout on the request
	ctx, cancel := context.WithTimeout(req.Context(), defaultRequestTimeout)
	defer cancel()
	req = req.WithContext(ctx)

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

func signatureHeaderValue(k crypto.Signer, challenge []byte) (string, error) {
	sig, err := echelper.SignWithTimeout(k, challenge, 1*time.Second, 250*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("could not sign challenge: %w", err)
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}
