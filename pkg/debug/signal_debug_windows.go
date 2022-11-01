package debug

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/Microsoft/go-winio"
	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
)

var pipePath = fmt.Sprintf(`\\.\pipe\launcher_debug_%s`, ulid.New())

func AttachDebugHandler(rootDir string, logger log.Logger) {
	if err := os.RemoveAll(pipePath); err != nil {
		logger.Log(
			"msg", "removing existing pipe",
			"pipe_path", pipePath,
			"err", err,
		)
		return
	}

	listener, err := winio.ListenPipe(pipePath, nil)
	if err != nil {
		logger.Log(
			"msg", "listening on pipe",
			"pipe_path", pipePath,
			"err", err,
		)
		return
	}

	go func() {
		defer listener.Close()

		var server *http.Server

		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Log(
					"msg", "accepting connection",
					"err", err,
				)
				return
			}
			defer conn.Close()

			// server not running
			if server == nil {
				debugServer, addr, err := startDebugServer(rootDir, logger)
				if err != nil {
					logger.Log(
						"msg", "starting debug server",
						"err", err,
					)
					server = nil
					conn.Write([]byte(fmt.Sprintf("error starting debug server: %s", err)))
					continue
				}

				server = debugServer
				conn.Write([]byte(fmt.Sprintf("debug server started at %s", addr)))
				continue
			}

			// server running
			err = server.Shutdown(context.Background())
			server = nil

			if err != nil {
				logger.Log(
					"msg", "shutting down debug server",
					"err", err,
				)

				continue
			}

			conn.Write([]byte("debug sever shutdown"))
		}
	}()
}

func ToggleDebugServer() (string, error) {
	conn, err := winio.DialPipe(pipePath, nil)
	if err != nil {
		return "", fmt.Errorf("dialing pipe %s: %w", pipePath, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte{})
	if err != nil {
		return "", fmt.Errorf("writing to pipe %s: %w", pipePath, err)
	}

	buf := make([]byte, 1024)
	mLen, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("reading debug toggle response: %w", err)
	}
	return string(buf[:mLen]), nil
}
