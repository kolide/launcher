// server is a http server that listens to a unix socket or named pipe for windows.
// Its implementation was driven by the need for "launcher proper" to be able to
// communicate with launcher desktop running as a separate process.
package server

import (
	"context"
	"net"
	"net/http"
	"os"

	"github.com/kolide/launcher/ee/desktop"
)

type DesktopServer struct {
	server       *http.Server
	listener     net.Listener
	shutdownChan chan<- struct{}
}

func New(shutdownChan chan<- struct{}) (*DesktopServer, error) {
	desktopServer := &DesktopServer{
		shutdownChan: shutdownChan,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/shutdown", desktopServer.shutdownHandler)

	desktopServer.server = &http.Server{
		Handler: mux,
	}

	socketPath := desktop.DesktopSocketPath(os.Getpid())

	// remove existing socket
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, err
	}

	listener, err := listener(socketPath)
	if err != nil {
		return nil, err
	}
	desktopServer.listener = listener

	desktopServer.server.RegisterOnShutdown(func() {
		// remove socket on shutdown
		if err := os.RemoveAll(socketPath); err != nil {
			//TODO: log this
		}
	})

	return desktopServer, nil
}

func (s *DesktopServer) Serve() error {
	return s.server.Serve(s.listener)
}

func (s *DesktopServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *DesktopServer) shutdownHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"msg\": \"shutting down\"}"))
	s.shutdownChan <- struct{}{}
}
