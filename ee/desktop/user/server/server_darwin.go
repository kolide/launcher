//go:build darwin
// +build darwin

package server

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
	"github.com/vmihailenco/msgpack/v5"
)

func (s *UserServer) createSecureEnclaveKey(w http.ResponseWriter, r *http.Request) {
	key, err := secureenclave.CreateKey()
	if err != nil {
		s.slogger.Log(r.Context(), slog.LevelDebug,
			"secure enclave unavailable, could not create key",
			"err", err,
		)
		http.Error(w, fmt.Errorf("secure enclave unavailable, could not create key: %w", err).Error(), http.StatusServiceUnavailable)
		return
	}

	keyBytes, err := echelper.PublicEcdsaToB64Der(key)
	if err != nil {
		http.Error(w, fmt.Errorf("serializing key: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	w.Write(keyBytes)
}

// getSecureEnclaveKey verifies that the public key exists in the secure enclave
// then returns the public key back to follow REST conventions
func (s *UserServer) getSecureEnclaveKey(w http.ResponseWriter, r *http.Request) {
	b64 := r.URL.Query().Get("pub_key")
	if b64 == "" {
		http.Error(w, "pub_key is required", http.StatusBadRequest)
		return
	}

	pubKey, err := echelper.PublicB64DerToEcdsaKey([]byte(b64))
	if err != nil {
		http.Error(w, fmt.Errorf("parsing pub_key: %w", err).Error(), http.StatusBadRequest)
		return
	}

	if err := isSecureEnclaveAvailable(); err != nil {
		s.slogger.Log(r.Context(), slog.LevelDebug,
			"secure enclave unavailable",
			"err", err,
		)
		http.Error(w, fmt.Errorf("secure enclave unavailable: %w", err).Error(), http.StatusServiceUnavailable)
		return
	}

	signer, err := secureenclave.New(pubKey)
	if err != nil {
		if !isKeyNotFoundErr(err) {
			// encountered some other error, cannot confirm if key exists
			http.Error(w, fmt.Errorf("encounter unexpected error, cannot determine if key exists in secure enclave: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		s.slogger.Log(r.Context(), slog.LevelInfo,
			"secure enclave key does not exist",
			"err", err,
		)
		http.Error(w, "key not found", http.StatusNotFound)
	}

	// try to convert public to ecdsa
	pubKey, ok := signer.Public().(*ecdsa.PublicKey)
	if !ok {
		http.Error(w, "public key is not ecdsa", http.StatusInternalServerError)
		return
	}

	keyBytes, err := echelper.PublicEcdsaToB64Der(pubKey)
	if err != nil {
		http.Error(w, fmt.Errorf("serializing key: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	w.Write(keyBytes)
}

func (s *UserServer) signWithSecureEnclave(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "request body is required", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var signRequest SignWithSecureEnclaveRequest
	if err := msgpack.NewDecoder(r.Body).Decode(&signRequest); err != nil {
		http.Error(w, fmt.Errorf("unmarshalling request body: %w", err).Error(), http.StatusBadRequest)
		return
	}

	pubKeyAny, err := x509.ParsePKIXPublicKey(signRequest.PubKeyDer)
	if err != nil {
		http.Error(w, fmt.Errorf("parsing pub_key_der: %w", err).Error(), http.StatusBadRequest)
		return
	}

	pubKey, ok := pubKeyAny.(*ecdsa.PublicKey)
	if !ok {
		http.Error(w, "pub_key_der is not ecdsa", http.StatusBadRequest)
		return
	}

	if err := isSecureEnclaveAvailable(); err != nil {
		s.slogger.Log(r.Context(), slog.LevelDebug,
			"secure enclave unavailable",
			"err", err,
		)
		http.Error(w, fmt.Errorf("secure enclave unavailable: %w", err).Error(), http.StatusServiceUnavailable)
		return
	}

	signer, err := secureenclave.New(pubKey)
	if err != nil {
		if !isKeyNotFoundErr(err) {
			// encountered some other error, cannot confirm if key exists
			http.Error(w, fmt.Errorf("encounter unexpected error, cannot determine if key exists in secure enclave: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		s.slogger.Log(r.Context(), slog.LevelInfo,
			"secure enclave key does not exist",
			"err", err,
		)
		http.Error(w, "key not found", http.StatusNotFound)
	}

	dataStr := string(signRequest.Data)
	// ensure these tags are on the data so that we can identify the response as a kolide response
	// and mitigate the agent signing arbitrary data
	if !strings.HasPrefix(dataStr, "kolide:") || !strings.HasSuffix(dataStr, ":kolide") {
		http.Error(w, "data must be prefixed with kolide: and suffixed with :kolide", http.StatusBadRequest)
		return
	}

	// sign data
	sig, err := echelper.SignWithTimeout(signer, signRequest.Data, 5*time.Second, 500*time.Millisecond)
	if err != nil {
		http.Error(w, fmt.Errorf("signing data: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	w.Write(sig)
}

func isSecureEnclaveAvailable() error {
	// we will try to create a key first to verify that we can access secure enclave
	testPubKey, err := secureenclave.CreateKey()
	if err != nil {
		return fmt.Errorf("secure enclave unavailable, could not create test key: %w", err)
	}

	// delete the test key
	if err := secureenclave.DeleteKey(testPubKey); err != nil {
		return fmt.Errorf("secure enclave unavailable, could not delete test key: %w", err)
	}

	return nil
}

func isKeyNotFoundErr(err error) bool {
	if err == nil {
		return false
	}

	// errKCItemNotFound = -25300
	// means item was not found, any other error we assume is a different problem
	// apple docs
	// https://developer.apple.com/documentation/coreservices/1559994-anonymous/errkcitemnotfound/
	return strings.Contains(err.Error(), "-25300")
}
