package localserver

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/observability"
)

const (
	dt4aAccountUuidHeaderKey = "X-DT4A-Account-Uuid"
	dt4aUserUuidHeaderKey    = "X-DT4A-User-Uuid"
)

var (
	// allowlistedDt4aOriginsLookup contains the complete list of origins that are permitted to access the /dt4a endpoint.
	allowlistedDt4aOriginsLookup = map[string]struct{}{
		// Release extension
		"chrome-extension://gejiddohjgogedgjnonbofjigllpkmbf":  {},
		"chrome-extension://khgocmkkpikpnmmkgmdnfckapcdkgfaf":  {},
		"chrome-extension://aeblfdkhhhdcdjpifhhbdiojplfjncoa":  {},
		"chrome-extension://dppgmdbiimibapkepcbdbmkaabgiofem":  {},
		"moz-extension://dfbae458-fb6f-4614-856e-094108a80852": {},
		"moz-extension://25fc87fa-4d31-4fee-b5c1-c32a7844c063": {},
		"moz-extension://d634138d-c276-4fc8-924b-40a0ea21d284": {},
		// Development and internal builds
		"chrome-extension://hjlinigoblmkhjejkmbegnoaljkphmgo":  {},
		"moz-extension://0a75d802-9aed-41e7-8daa-24c067386e82": {},
		"chrome-extension://hiajhnnfoihkhlmfejoljaokdpgboiea":  {},
		"chrome-extension://kioanpobaefjdloichnjebbdafiloboa":  {},
		"chrome-extension://bkpbhnjcbehoklfkljkkbbmipaphipgl":  {},
		// Development web app
		"https://my.b5local.com:4000":           {},
		"https://dev.sites.gitlab.1password.io": {},
	}

	allowlisted1POriginRegex = regexp.MustCompile(`https:\/\/.+\.1password\.(com|ca|eu)`)
)

const (
	safariWebExtensionScheme = "safari-web-extension://"
)

type dt4aAuthMiddleware struct {
	// counterPartyKeys is a map of trusted keys, maps KID to pubkey
	counterPartyKeys map[string]*ecdsa.PublicKey
	slogger          *slog.Logger
}

type dt4aResponse struct {
	Data   string `json:"data"`
	PubKey string `json:"pubKey"`
}

// originIsAllowlisted checks the given request origin against our allowable values.
// We allow present-but-empty origins.
func originIsAllowlisted(requestOrigin string) bool {
	// Allow present-but-empty origins
	if requestOrigin == "" {
		return true
	}

	// Allow origins in the allowlist
	if _, ok := allowlistedDt4aOriginsLookup[requestOrigin]; ok {
		return true
	}

	// Allow origin from safari web extension
	if strings.HasPrefix(requestOrigin, safariWebExtensionScheme) {
		return true
	}

	// Check against known/allowlisted origin patterns
	if allowlisted1POriginRegex.MatchString(requestOrigin) {
		return true
	}

	return false
}

func (d *dt4aAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, span := observability.StartHttpRequestSpan(r)
		defer span.End()

		// Validate origin. We expect to either have the origin set to an allowlisted value, or to be
		// present but empty, or to be missing. We will not allow a request with a nonempty origin
		// that is not in the allowlist.
		requestOrigin := r.Header.Get("Origin")
		if !originIsAllowlisted(requestOrigin) {
			escapedOrigin := strings.ReplaceAll(strings.ReplaceAll(requestOrigin, "\n", ""), "\r", "") // remove any newlines
			d.slogger.Log(r.Context(), slog.LevelInfo,
				"received dt4a request with origin not in allowlist",
				"req_origin", escapedOrigin,
			)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		boxParam := r.URL.Query().Get("payload")
		if boxParam == "" {
			d.slogger.Log(r.Context(), slog.LevelWarn,
				"missing payload url param",
			)

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		boxBytes, err := base64.URLEncoding.DecodeString(boxParam)
		if err != nil {
			d.slogger.Log(r.Context(), slog.LevelWarn,
				"failed to decode payload",
				"err", err,
			)

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var requestTrustChain chain
		if err := json.Unmarshal(boxBytes, &requestTrustChain); err != nil {
			d.slogger.Log(r.Context(), slog.LevelWarn,
				"failed to unmarshal chain",
				"err", err,
			)

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := requestTrustChain.validate(d.counterPartyKeys); err != nil {
			d.slogger.Log(r.Context(), slog.LevelWarn,
				"failed to validate chain",
				"err", err,
			)

			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		bhrHeaders := make(http.Header)
		maps.Copy(bhrHeaders, r.Header)

		bhrHeaders.Add(dt4aAccountUuidHeaderKey, requestTrustChain.accountUuid)
		bhrHeaders.Add(dt4aUserUuidHeaderKey, requestTrustChain.userUuid)

		newReq := &http.Request{
			Method: r.Method,
			Header: bhrHeaders,
			URL: &url.URL{
				Scheme: r.URL.Scheme,
				Host:   r.Host,
				// remove version the as it only designates the auth version
				Path: strings.TrimPrefix(r.URL.Path, "/v3"),
			},
		}

		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		if bhr.code < 200 || bhr.code >= 300 {
			// hacky sleep here so that it's likely a response form another launcher
			// on a different port will return with success faster than this failure
			time.Sleep(100 * time.Millisecond)
		}

		// even if the downstream hander did not write a body, we still create a nacl box & dt4a response
		// this allows a caller to know it's at least talking to valid agent
		box, pubKey, err := echelper.SealNaCl(bhr.Bytes(), requestTrustChain.counterPartyPubEncryptionKey)
		if err != nil {
			d.slogger.Log(r.Context(), slog.LevelError,
				"failed to seal response",
				"err", err,
			)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		dt4aResponse := dt4aResponse{
			Data:   base64.URLEncoding.EncodeToString(box),
			PubKey: base64.URLEncoding.EncodeToString(pubKey[:]),
		}

		dt4aResponseJson, err := json.Marshal(dt4aResponse)
		if err != nil {
			d.slogger.Log(r.Context(), slog.LevelError,
				"failed to marshal dt4a response",
				"err", err,
			)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// if a downstream handler has set a code, set it here
		if bhr.code != 0 {
			w.WriteHeader(bhr.code)
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(dt4aResponseJson)
	})
}

type payload struct {
	AccountUuid    string `json:"accountUuid"`
	DateTimeSigned int64  `json:"dateTimeSigned"`
	Environment    string `json:"environment"`
	ExpirationDate int64  `json:"expirationDate"`
	PublicKey      jwk    `json:"publicKey"`
	SignedBy       string `json:"signedBy"`
	UserUuid       string `json:"userUuid"`
	Version        uint8  `json:"version"`
}

// chain represents a chain of trust, where each link in the chain has a payload signed by the key in the previous link.
// The first link in the chain is signed by a trusted root key.
// There can be any number of keys between the root and the last key.
// The last key is an x25519 key which is the public half of a NaCl box key.
type chain struct {
	Links []chainLink `json:"links"`
	// counterPartyPubEncryptionKey is the last public key in the chain, which is a x25519 key
	// we set this after we extract and verify in the validate method
	counterPartyPubEncryptionKey *[32]byte

	// userdata set using verified data from last link in chain
	accountUuid, userUuid string
}

// chainLink represents a link in a chain of trust
type chainLink struct {
	// Payload a b64 url encoded json
	Payload string `json:"payload"`
	// Signature is b64 url encode of signature
	Signature string `json:"signature"`
	// Signed by is the key id "kid" of the key that signed this link
	SignedBy string `json:"signedBy"`
}

func (c *chain) UnmarshalJSON(data []byte) error {
	// incomming data is just an array of chain links
	var links []chainLink
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to unmarshal chain links: %w", err)
	}

	c.Links = links
	return nil
}

// MarshalJSON marshals the chain as a json array of chain links
func (c *chain) MarshalJSON() ([]byte, error) {
	// outgoing data is just an aray of chain links
	bytes, err := json.Marshal(c.Links)

	if err != nil {
		return nil, fmt.Errorf("failed to marshal chain links: %w", err)
	}
	return bytes, nil
}

// validate iterates over all the links in the chain and verifies the signatures and timestamps
// the root p256 ecdsa key must be in the trusted keys map, the map key is the "kid" of the key
func (c *chain) validate(trustedKeys map[string]*ecdsa.PublicKey) error {
	if len(c.Links) == 0 {
		return errors.New("chain is empty")
	}

	for i := range len(c.Links) {
		if c.Links[i].Payload == "" || c.Links[i].Signature == "" {
			return fmt.Errorf("link %d is missing data or sig", i)
		}
	}

	rootKey, ok := trustedKeys[c.Links[0].SignedBy]
	if !ok {
		return fmt.Errorf("root key with kid %s not found in trusted keys", c.Links[0].SignedBy)
	}

	// make a copy of the root key so that we can reassign it
	parentEcdsa := &ecdsa.PublicKey{
		Curve: rootKey.Curve,
		X:     rootKey.X,
		Y:     rootKey.Y,
	}

	var currentPayload payload

	for i := range len(c.Links) {

		signature, err := base64.URLEncoding.DecodeString(c.Links[i].Signature)
		if err != nil {
			return fmt.Errorf("failed to decode signature at index %d: %w", i, err)
		}

		if err := echelper.VerifySignature(parentEcdsa, []byte(c.Links[i].Payload), signature); err != nil {
			return fmt.Errorf("invalid signature at index %d: %w", i, err)
		}

		payloadBytes, err := base64.URLEncoding.DecodeString(c.Links[i].Payload)
		if err != nil {
			return fmt.Errorf("failed to decode payload of index %d: %w", i, err)
		}

		if err := json.Unmarshal(payloadBytes, &currentPayload); err != nil {
			return fmt.Errorf("failed to unmarshal payload of index %d: %w", i, err)
		}

		if currentPayload.ExpirationDate < time.Now().Unix() {
			return fmt.Errorf("payload at index %d has expired, kid %s", i, currentPayload.PublicKey.KeyID)
		}

		// always set to last verified link in chain, no particular reason to use last link
		// all links should have same value
		c.accountUuid = currentPayload.AccountUuid
		c.userUuid = currentPayload.UserUuid

		// last link is a x25519 key no used for signature validation, no need to convert / set as parent
		if i < len(c.Links)-1 {
			ecdsaPubKey, err := currentPayload.PublicKey.ecdsaPubKey()
			if err != nil {
				return fmt.Errorf("failed to convert public key at index %d, kid %s into public ecdsa key: %w", i, currentPayload.PublicKey.KeyID, err)
			}

			parentEcdsa = ecdsaPubKey
		}
	}

	rawX25519Key, err := currentPayload.PublicKey.x25519PubKey()
	if err != nil {
		return fmt.Errorf("failed to convert last public key into x25519 key: %w", err)
	}

	// set the counter party key
	c.counterPartyPubEncryptionKey = rawX25519Key

	return nil
}
