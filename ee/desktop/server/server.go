package server

import (
	"context"
	"net/http"
	"os"

	"fyne.io/systray"
	"github.com/kolide/launcher/ee/desktop"
)

var shutdownChan = make(chan struct{})

func Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/shutdown", shutdown)

	server := http.Server{
		Handler: mux,
	}

	socketPath := desktop.DesktopSocketPath(os.Getpid())

	if err := os.RemoveAll(socketPath); err != nil {
		//TODO: log this
	}

	listener, err := listener(socketPath)
	if err != nil {
		//TODO: log this
	}

	go func() {
		if err := server.Serve(listener); err != nil {
			// TODO: log this
			shutdownChan <- struct{}{}
		}
	}()

	<-shutdownChan

	if err := os.RemoveAll(socketPath); err != nil {
		//TODO: log this
	}

	if err := server.Shutdown(context.Background()); err != nil {
		//TODO: log this
	}

	systray.Quit()
	return nil
}

func shutdown(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"msg\": \"shutting down\"}"))
	shutdownChan <- struct{}{}
}
