//go:build darwin
// +build darwin

package server

import (
	"fmt"
	"net/http"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/krypto/pkg/secureenclave"
)

func (s *UserServer) createSecureEnclaveKey(w http.ResponseWriter, _ *http.Request) {
	key, err := secureenclave.CreateKey()
	if err != nil {
		http.Error(w, fmt.Errorf("creating key: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	keyBytes, err := echelper.PublicEcdsaToB64Der(key)
	if err != nil {
		http.Error(w, fmt.Errorf("serializing key: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	w.Write(keyBytes)
	w.WriteHeader(http.StatusOK)
}
