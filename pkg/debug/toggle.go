package debug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/socket"
)

var debugServer *http.Server

func AttachDebugHandler(rootDir string, logger log.Logger) {
	socketPath, err := writeSocketPath(rootDir)
	if err != nil {
		logger.Log(
			"msg", "writing socket path",
			"err", err,
		)
		return
	}

	if err := os.RemoveAll(socketPath); err != nil {
		logger.Log(
			"msg", "removing existing socket",
			"pipe_path", socketPath,
			"err", err,
		)
		return
	}

	listener, err := socket.Listen(socketPath)
	if err != nil {
		logger.Log(
			"msg", "listening on socket",
			"pipe_path", socketPath,
			"err", err,
		)
		return
	}

	go func() {
		defer listener.Close()

		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Log(
					"msg", "accepting connection",
					"err", err,
				)
				continue
			}

			// in most cases this would ge a go routine, but I think in this case we want this to be
			// synchronous since it's a toggle
			handleConnection(logger, conn, rootDir)
		}
	}()
}

func handleConnection(logger log.Logger, conn net.Conn, rootDir string) {
	defer conn.Close()

	// server not running
	if debugServer == nil {
		server, addr, err := startDebugServer(filepath.Join(rootDir, "debug_addr"), logger)
		if err != nil {
			logger.Log(
				"msg", "starting debug server",
				"err", err,
			)
			server = nil
			conn.Write([]byte(fmt.Sprintf("error starting debug server: %s", err)))
			return
		}

		debugServer = server
		conn.Write([]byte(addr))
		return
	}

	// server running
	err := debugServer.Shutdown(context.Background())
	debugServer = nil

	if err != nil {
		logger.Log(
			"msg", "shutting down debug server",
			"err", err,
		)

		return
	}

	conn.Write([]byte("debug sever shutdown"))
}

func ToggleDebugServer(rootDir string) (string, error) {
	socketPath, err := readSocketPath(rootDir)
	if err != nil {
		return "", fmt.Errorf("reading socket path: %w", err)
	}

	conn, err := socket.Dial(socketPath)
	if err != nil {
		return "", fmt.Errorf("dialing socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte{})
	if err != nil {
		return "", fmt.Errorf("writing to socket %s: %w", socketPath, err)
	}

	buf := make([]byte, 1024)
	mLen, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("reading debug toggle response: %w", err)
	}
	return string(buf[:mLen]), nil
}

// writeSocketPath returns the path to the socket and writes the path to the socketPathFilePath().
// Since the socket file has a random ulid attached to it, the socket path file can be used by
// other processes to locate the file.
func writeSocketPath(rootDir string) (string, error) {
	socketPath := filepath.Join(rootDir, fmt.Sprintf("debug_socket_%s", ulid.New()))

	if runtime.GOOS == "windows" {
		socketPath = fmt.Sprintf(`\\.\pipe\launcher_debug_%s`, ulid.New())
	}

	if err := os.WriteFile(socketPathFilePath(rootDir), []byte(socketPath), 0600); err != nil {
		return "", fmt.Errorf("writing socket path file: %w", err)
	}

	return socketPath, nil
}

// readSocketPath returns the path to the debug enable socket found in
// the contents of the file at socketPathFilePath()
func readSocketPath(rootDir string) (string, error) {
	socketPathFilePath := socketPathFilePath(rootDir)

	if _, err := os.Stat(socketPathFilePath); err != nil {
		return "", fmt.Errorf("socket path file does not exist: %w", err)
	}

	contents, err := os.ReadFile(socketPathFilePath)
	if err != nil {
		return "", fmt.Errorf("reading socket path file: %w", err)
	}

	return string(contents), nil
}

func socketPathFilePath(rootDir string) string {
	return filepath.Join(rootDir, "debug_enable_socket_path")
}
