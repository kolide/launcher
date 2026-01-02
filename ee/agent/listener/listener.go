package listener

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/ee/agent/types"
)

const (
	RootLauncherListenerSocketPrefix = "root_launcher"
)

// launcherListener is a rungroup actor that creates a socket and listens on it.
// This allows sufficiently-authenticated processes to communicate with the root
// launcher process.
type launcherListener struct {
	slogger     *slog.Logger
	k           types.Knapsack
	socketPath  string
	listener    net.Listener
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

func NewLauncherListener(k types.Knapsack, slogger *slog.Logger, socketPrefix string) (*launcherListener, error) {
	listenerSlogger := slogger.With("component", "launcher_listener", "socket_prefix", socketPrefix)
	netListener, socketPath, err := initSocket(k, listenerSlogger, socketPrefix)
	if err != nil {
		return nil, fmt.Errorf("initializing socket: %w", err)
	}
	return &launcherListener{
		slogger:     listenerSlogger,
		k:           k,
		socketPath:  socketPath,
		listener:    netListener,
		interrupt:   make(chan struct{}, 1), // Buffer so that Interrupt can send to this channel and return, even if Execute has already terminated
		interrupted: &atomic.Bool{},
	}, nil
}

func initSocket(k types.Knapsack, slogger *slog.Logger, socketPrefix string) (net.Listener, string, error) {
	// First, find and remove any existing sockets with the same prefix
	socketPrefixWithPath := filepath.Join(k.RootDirectory(), socketPrefix)
	matches, err := filepath.Glob(socketPrefixWithPath + "*")
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"could not glob for existing sockets",
			"err", err,
		)
	} else {
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				slogger.Log(context.TODO(), slog.LevelWarn,
					"removing existing socket",
					"path", match,
					"err", err,
				)
			}
		}
	}

	// Now, create new pipe -- we use a random 4-digit number over ulid to avoid too-long paths.
	socketPath := fmt.Sprintf("%s_%d", socketPrefixWithPath, rand.Intn(10000))
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, socketPath, fmt.Errorf("listening at %s: %w", socketPath, err)
	}

	// Ensure the permissions are set correctly for the socket -- we require root/admin.
	if err := setSocketPermissions(socketPath); err != nil {
		listener.Close()
		return nil, socketPath, fmt.Errorf("setting appropriate permissions on %s: %w", socketPath, err)
	}

	return listener, socketPath, nil
}

func (l *launcherListener) Execute() error {
	// Repeatedly check for new connections. We handle one connection at a time.
	for {
		var conn net.Conn
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.interrupt:
				l.slogger.Log(context.TODO(), slog.LevelDebug,
					"received shutdown, exiting loop",
				)
				return nil
			default:
				l.slogger.Log(context.TODO(), slog.LevelError,
					"could not accept incoming connection",
					"err", err,
				)
				continue
			}
		}

		l.slogger.Log(context.TODO(), slog.LevelInfo,
			"opened connection",
		)

		if err := l.handleConn(conn); err != nil {
			l.slogger.Log(context.TODO(), slog.LevelError,
				"could not handle incoming connection",
				"err", err,
			)
		}
	}
}

// handleConn handles the lifecycle of the incoming connection -- processing messages,
// sending responses as necessary, and closing the connection.
func (l *launcherListener) handleConn(conn net.Conn) error {
	// Ensure we close connection after we're done with it
	defer func() {
		if err := conn.Close(); err != nil {
			l.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not close connection",
				"err", err,
			)
		}
	}()

	// Read in the incoming message
	jsonReader := json.NewDecoder(conn)
	var msg launcherMessage
	if err := jsonReader.Decode(&msg); err != nil {
		return fmt.Errorf("decoding incoming message: %w", err)
	}

	switch msg.Type {
	case messageTypeEnroll:
		var e enrollmentAction
		if err := json.Unmarshal(msg.MsgData, &e); err != nil {
			return fmt.Errorf("unmarshalling enrollment message data: %w", err)
		}
		if err := l.handleEnrollmentAction(e); err != nil {
			return fmt.Errorf("handling enrollment: %w", err)
		}
	default:
		return fmt.Errorf("unsupported message type %s", msg.Type)
	}

	return nil
}

func (l *launcherListener) handleEnrollmentAction(e enrollmentAction) error {
	// Do a small amount of validation for the JWT. We do not have the key, and thus cannot fully verify --
	// so we use ParseUnverified. The cloud will handle full verification.
	token, _, err := new(jwt.Parser).ParseUnverified(e.EnrollmentSecret, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("parsing enrollment secret: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("no claims in enrollment secret")
	}
	munemoClaim, munemoFound := claims["organization"]
	if !munemoFound {
		return errors.New("invalid enrollment secret")
	}

	// Now that we're satisfied with the JWT, kick off enrollment.
	l.slogger.Log(context.TODO(), slog.LevelInfo,
		"processing request to enroll",
		"munemo", fmt.Sprintf("%s", munemoClaim),
	)

	// TODO RM: store secret in store; update knapsack to read secret from store additionally

	// TODO RM: pass in extension and call Enroll

	// TODO RM: need to send response back to conn appropriately
	return nil
}

func (l *launcherListener) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if l.interrupted.Swap(true) {
		return
	}

	// Shut down listener
	l.interrupt <- struct{}{}
	if err := l.listener.Close(); err != nil {
		l.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not close listener during interrupt",
			"err", err,
		)
	}
}
