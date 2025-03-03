//go:build !darwin
// +build !darwin

package server

import "net/http"

func (s *UserServer) createSecureEnclaveKey(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented on non darwin", http.StatusNotImplemented)
}

func (s *UserServer) getSecureEnclaveKey(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented on non darwin", http.StatusNotImplemented)
}
