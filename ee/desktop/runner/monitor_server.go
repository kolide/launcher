package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/kolide/kit/ulid"
)

type monitorServer struct {
	server   *http.Server
	listener net.Listener
	mux      *http.ServeMux
}

func newMonitorServer() (*monitorServer, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}

	ms := &monitorServer{
		mux:      &http.ServeMux{},
		listener: listener,
	}

	ms.server = &http.Server{
		Handler: ms.mux,
	}

	return ms, err
}

func (ms *monitorServer) serve() error {
	return ms.server.Serve(ms.listener)
}

func (ms *monitorServer) Shutdown(ctx context.Context) error {
	return ms.server.Shutdown(ctx)
}

func noOpHandleFunc(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		r.Body.Close()
	}
}

func (ms *monitorServer) newEndpoint() string {
	endpoint := fmt.Sprintf("/%s", ulid.New())
	ms.mux.HandleFunc(endpoint, noOpHandleFunc)
	return fmt.Sprintf("http://%s%s", ms.listener.Addr().String(), endpoint)
}
