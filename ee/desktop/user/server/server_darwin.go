//go:build darwin
// +build darwin

package server

import (
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
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

	// we will try to create a key first to verify that we can access secure enclave
	testPubKey, err := secureenclave.CreateKey()
	if err != nil {
		s.slogger.Log(r.Context(), slog.LevelDebug,
			"secure enclave unavailable, could not create test key",
			"err", err,
		)

		http.Error(w, fmt.Errorf("secure enclave unavailable, could not create test key: %w", err).Error(), http.StatusServiceUnavailable)
		return
	}

	if err := secureenclave.DeleteKey(testPubKey); err != nil {
		// this is unlikely, but if we fail to delete now, we should probalby assume something is wrong
		s.slogger.Log(r.Context(), slog.LevelError,
			"secure enclave unavailable, could not delete test key",
			"err", err,
		)
		http.Error(w, fmt.Errorf("secure enclave unavailable, could not delete test key: %w", err).Error(), http.StatusServiceUnavailable)
		return
	}

	// this verifies that the key exists in the secure enclave
	signer, err := secureenclave.New(pubKey)
	if err != nil {
		// errKCItemNotFound = -25300
		// means item was not found, any other error we assume is a different problem
		// apple docs
		// https://developer.apple.com/documentation/coreservices/1559994-anonymous/errkcitemnotfound/
		// apple site where you can search for error codes, just enter error code
		// https://developer.apple.com/bugreporter/
		if strings.Contains(err.Error(), "-25300") {
			s.slogger.Log(r.Context(), slog.LevelInfo,
				"secure enclave key does not exist",
				"err", err,
			)
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}

		s.slogger.Log(r.Context(), slog.LevelError,
			"encountered unexpected error, cannot determine if key exists in secure enclave",
			"err", err,
		)

		// encountered some other error, cannot confirm if key exists
		http.Error(w, fmt.Errorf("encounter unexpected error, cannot determine if key exists in secure enclave: %w", err).Error(), http.StatusInternalServerError)
		return
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
