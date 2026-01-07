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
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/ee/agent/storage"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
)

const (
	RootLauncherListenerSocketPrefix = "root_launcher"
	readTimeout                      = 10 * time.Second
	writeTimeout                     = 10 * time.Second
	enrollTimeout                    = 2 * time.Minute // up to 1 min to fetch enroll details, up to 30 sec to make enroll request, plus a buffer
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
	listener, err := net.Listen("unix", socketPath) //nolint:noctx // will fix in https://github.com/kolide/launcher/pull/2526
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
				"error handling connection",
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

	// Ensure we send a response
	var resp response
	defer func() {
		rawResp, err := json.Marshal(resp)
		if err != nil {
			l.slogger.Log(context.TODO(), slog.LevelError,
				"could not marshal response",
				"err", err,
			)
			return
		}
		// Ensure we don't block forever waiting to write
		if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
			l.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not set write deadline",
				"err", err,
			)
		}
		if _, err := conn.Write(rawResp); err != nil {
			l.slogger.Log(context.TODO(), slog.LevelError,
				"could not write response",
				"err", err,
			)
		}
	}()

	// Read in the incoming message
	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		l.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not set read deadline",
			"err", err,
		)
	}
	jsonReader := json.NewDecoder(conn)
	var req request
	if err := jsonReader.Decode(&req); err != nil {
		return fmt.Errorf("decoding incoming request: %w", err)
	}

	switch req.Type {
	case messageTypeEnroll:
		var e enrollmentRequest
		if err := json.Unmarshal(req.Data, &e); err != nil {
			resp.Success = false
			resp.Message = fmt.Sprintf("request is not valid JSON: %v", err)
			return fmt.Errorf("unmarshalling enrollment request data: %w", err)
		}
		if err := l.handleEnrollmentRequest(e); err != nil {
			resp.Success = false
			resp.Message = fmt.Sprintf("could not perform enrollment: %v", err)
			return fmt.Errorf("handling enrollment: %w", err)
		}
		resp.Success = true
	default:
		resp.Success = false
		resp.Message = fmt.Sprintf("unsupported request type %s", req.Type)
		return fmt.Errorf("unsupported request type %s", req.Type)
	}

	return nil
}

func (l *launcherListener) handleEnrollmentRequest(e enrollmentRequest) error {
	// For now, don't perform enrollment if already enrolled. We may change this behavior
	// when we tackle multitenancy.
	currentEnrollmentStatus, err := l.k.CurrentEnrollmentStatus()
	if err != nil {
		return fmt.Errorf("determining current enrollment status: %w", err)
	}
	if currentEnrollmentStatus == types.Enrolled {
		return errors.New("already enrolled")
	}

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

	// Store the enrollment secret in our token store, so that the extension can pick it up.
	// For now, we store it under the "default" enrollment.
	tokenStore := l.k.TokenStore()
	if tokenStore == nil {
		// Should never happen, but we check just in case
		return errors.New("token store not available")
	}
	if err := tokenStore.Set(storage.KeyByIdentifier(storage.EnrollmentSecretTokenKey, storage.IdentifierTypeRegistration, []byte(types.DefaultRegistrationID)), []byte(e.EnrollmentSecret)); err != nil {
		return fmt.Errorf("storing enrollment secret: %w", err)
	}

	// Now that the secret is set and available, the osquery extension will attempt to enroll on the next
	// request (likely within 5 seconds). Wait for the enrollment to complete.
	if err := backoff.WaitFor(func() error {
		currentEnrollmentStatus, err := l.k.CurrentEnrollmentStatus()
		if err != nil {
			return fmt.Errorf("determining current enrollment status: %w", err)
		}
		if currentEnrollmentStatus != types.Enrolled {
			return fmt.Errorf("enroll has not yet completed (status %s)", currentEnrollmentStatus)
		}
		return nil
	}, enrollTimeout, 5*time.Second); err != nil {
		return fmt.Errorf("enrollment not successful before timeout: %w", err)
	}

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
