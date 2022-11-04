package debug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/socket"
	"github.com/kolide/launcher/pkg/backoff"
)

type DebugToggleListener struct {
	logger      log.Logger
	rootDir     string
	socketPath  string
	listener    net.Listener
	debugServer *http.Server
}

func NewDebugToggleListener(logger log.Logger, rootDir string) *DebugToggleListener {
	return &DebugToggleListener{
		logger:  logger,
		rootDir: rootDir,
	}
}

func (d *DebugToggleListener) Listen() error {
	socketPath, err := writeSocketPath(d.rootDir)
	if err != nil {
		d.logger.Log(
			"msg", "writing socket path",
			"err", err,
		)
		return err
	}

	d.socketPath = socketPath

	if err := os.RemoveAll(socketPath); err != nil {
		d.logger.Log(
			"msg", "removing existing socket",
			"socket_path", socketPath,
			"err", err,
		)
		return err
	}

	listener, err := socket.Listen(socketPath)
	if err != nil {
		d.logger.Log(
			"msg", "listening on socket",
			"socket_path", socketPath,
			"err", err,
		)
		return err
	}
	defer listener.Close()
	d.listener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			d.logger.Log(
				"msg", "accepting connection",
				"err", err,
			)
			continue
		}

		// in most cases this would be a go routine, but I think in this case we want this to be
		// synchronous since it's a toggle
		d.handleConnection(conn)
		conn.Close()
	}
}

func (d *DebugToggleListener) Shutdown() error {
	level.Debug(d.logger).Log(
		"msg", "shutting down debug toggle listener",
	)

	return backoff.WaitFor(func() error {
		// only close on windows, gives error on posix
		if runtime.GOOS == "windows" {
			if err := d.listener.Close(); err != nil {
				return fmt.Errorf("closing listener: %w", err)
			}
		}

		filesToRemove := []string{
			socketPathFilePath(d.rootDir),
			d.socketPath,
		}

		for _, file := range filesToRemove {
			if err := os.RemoveAll(file); err != nil {
				return fmt.Errorf("removing file %s: %w", file, err)
			}
		}

		return nil
	}, 5*time.Second, 1*time.Second)
}

func (d *DebugToggleListener) handleConnection(conn net.Conn) {
	// server not running
	if d.debugServer == nil {
		debugServer, addr, err := startDebugServer(filepath.Join(d.rootDir, "debug_addr"), d.logger)
		if err != nil {
			d.logger.Log(
				"msg", "starting debug server",
				"err", err,
			)
			d.debugServer = nil
			conn.Write([]byte(fmt.Sprintf("error starting debug server: %s", err)))
			return
		}

		d.debugServer = debugServer
		conn.Write([]byte(addr))
		return
	}

	// server running
	err := d.debugServer.Shutdown(context.Background())
	d.debugServer = nil

	if err != nil {
		d.logger.Log(
			"msg", "shutting down debug server",
			"err", err,
		)

		return
	}

	conn.Write([]byte("debug sever shutdown"))
}

func ToggleDebug(rootDir string) (string, error) {
	var result string

	err := backoff.WaitFor(func() error {
		ret, err := sendOnDebugToggleSocket(rootDir)
		if err != nil {
			return err
		}
		result = ret
		return nil
	}, 5*time.Second, 1*time.Second)

	return result, err
}

func sendOnDebugToggleSocket(rootDir string) (string, error) {
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
// Since the socket file on windows has a random ulid attached to it, the socket path file can be used by
// other processes to locate the socket file.
func writeSocketPath(rootDir string) (string, error) {
	socketPath := filepath.Join(rootDir, "debug_toggle_socket")

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
