package localserver

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/osquery"
	"go.etcd.io/bbolt"
	"golang.org/x/time/rate"
)

// Special Kolide Ports
var portList = []int{
	12519,
	40978,
	52115,
	22287,
	60685,
	22322,
}

type Querier interface {
	Query(query string) ([]map[string]string, error)
}

type localServer struct {
	logger       log.Logger
	srv          *http.Server
	identifiers  identifiers
	limiter      *rate.Limiter
	tlsCerts     []tls.Certificate
	querier      Querier
	kolideServer string

	myKey                 *rsa.PrivateKey
	myLocalDbSigner       crypto.Signer
	myLocalHardwareSigner crypto.Signer

	serverKey   *rsa.PublicKey
	serverEcKey *ecdsa.PublicKey
}

const (
	defaultRateLimit = 5
	defaultRateBurst = 10
)

func New(logger log.Logger, db *bbolt.DB, kolideServer string) (*localServer, error) {
	ls := &localServer{
		logger:                log.With(logger, "component", "localserver"),
		limiter:               rate.NewLimiter(defaultRateLimit, defaultRateBurst),
		kolideServer:          kolideServer,
		myLocalDbSigner:       agent.LocalDbKeys(),
		myLocalHardwareSigner: agent.HardwareKeys(),
	}

	// TODO: As there may be things that adjust the keys during runtime, we need to persist that across
	// restarts. We should load-old-state here. This is still pretty TBD, so don't angst too much.
	if err := ls.LoadDefaultKeyIfNotSet(); err != nil {
		return nil, err
	}

	// Consider polling this on an interval, so we get updates.
	privateKey, err := osquery.PrivateRSAKeyFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("fetching private key: %w", err)
	}
	ls.myKey = privateKey

	ecKryptoMiddleware := newKryptoEcMiddleware(ls.logger, ls.myLocalDbSigner, ls.myLocalHardwareSigner, *ls.serverEcKey)
	ecAuthedMux := http.NewServeMux()
	ecAuthedMux.HandleFunc("/", http.NotFound)
	ecAuthedMux.Handle("/id", ls.requestIdHandler())
	ecAuthedMux.Handle("/id.png", ls.requestIdHandler())
	ecAuthedMux.Handle("/query", ls.requestQueryHandler())
	ecAuthedMux.Handle("/query.png", ls.requestQueryHandler())
	ecAuthedMux.Handle("/runscheduledquery", ls.requestQueryHandler())
	ecAuthedMux.Handle("/runscheduledquery.png", ls.requestQueryHandler())

	mux := http.NewServeMux()
	mux.HandleFunc("/", http.NotFound)
	mux.Handle("/v0/cmd", ecKryptoMiddleware.Wrap(ecAuthedMux))

	// uncomment to test without going through middleware
	// for example:
	// curl localhost:40978/query --data '{"query":"select * from kolide_launcher_info"}'
	// mux.Handle("/query", ls.requestQueryHandler())
	// curl localhost:40978/runscheduledquery --data '{"name":"pack:kolide_device_updaters:agentprocesses-all:snapshot"}'
	// mux.Handle("/runscheduledquery", ls.requestRunScheduledQueryHandler())

	srv := &http.Server{
		Handler:           ls.requestLoggingHandler(ls.preflightCorsHandler(ls.rateLimitHandler(mux))),
		ReadTimeout:       500 * time.Millisecond,
		ReadHeaderTimeout: 50 * time.Millisecond,
		WriteTimeout:      5 * time.Second,
		MaxHeaderBytes:    1024,
	}

	ls.srv = srv

	return ls, nil
}

func (ls *localServer) SetQuerier(querier Querier) {
	ls.querier = querier
}

func (ls *localServer) LoadDefaultKeyIfNotSet() error {
	if ls.serverKey != nil {
		return nil
	}

	serverRsaCertPem := k2RsaServerCert
	serverEccCertPem := k2EccServerCert
	switch {
	case strings.HasPrefix(ls.kolideServer, "localhost"), strings.HasPrefix(ls.kolideServer, "127.0.0.1"):
		serverRsaCertPem = localhostRsaServerCert
		serverEccCertPem = localhostEccServerCert
	case strings.HasSuffix(ls.kolideServer, ".herokuapp.com"):
		serverRsaCertPem = reviewRsaServerCert
		serverEccCertPem = reviewEccServerCert
	}

	serverKeyRaw, err := krypto.KeyFromPem([]byte(serverRsaCertPem))
	if err != nil {
		return fmt.Errorf("parsing default public key: %w", err)
	}

	serverKey, ok := serverKeyRaw.(*rsa.PublicKey)
	if !ok {
		return errors.New("public key not an rsa public key")
	}

	ls.serverKey = serverKey

	ls.serverEcKey, err = echelper.PublicPemToEcdsaKey([]byte(serverEccCertPem))
	if err != nil {
		return fmt.Errorf("parsing default server ec key: %w", err)
	}

	return nil
}

func (ls *localServer) runAsyncdWorkers() time.Time {
	success := true

	level.Debug(ls.logger).Log("msg", "Starting an async worker run")

	if err := ls.updateIdFields(); err != nil {
		success = false
		level.Info(ls.logger).Log(
			"msg", "Got error updating id fields",
			"err", err,
		)
	}

	level.Debug(ls.logger).Log(
		"msg", "Completed async worker run",
		"success", success,
	)

	if !success {
		return time.Time{}
	}
	return time.Now()
}

func (ls *localServer) Start() error {
	// Spawn background workers. This loop is a bit weird on startup. We want to populate this data as soon as we can, but because the underlying launcher
	// run group isn't ordered, this is likely to happen before querier is ready. So we retry at a frequent interval for a couple of minutes, then we drop
	// back to a slower poll interval. Note that this polling is merely a check against time, we don't repopulate this data nearly so often. (But we poll
	// frequently to account for the difference between wall clock time, and sleep time)
	const (
		initialPollInterval = 10 * time.Second
		initialPollTimeout  = 2 * time.Minute
		pollInterval        = 15 * time.Minute
		recalculateInterval = 24 * time.Hour
	)
	go func() {
		// Initial load, run pretty often, at least for the first chunk of time.
		var lastRun time.Time
		if err := backoff.WaitFor(func() error {
			lastRun = ls.runAsyncdWorkers()
			if (lastRun == time.Time{}) {
				return errors.New("async tasks not success on initial boot (no surprise)")
			}
			return nil
		},
			initialPollTimeout,
			initialPollInterval,
		); err != nil {
			level.Info(ls.logger).Log("message", "Initial async runs unsuccessful. Will retry in the future.", "err", err)
		}

		// Now that we're done with the initial population, fall back to a periodic polling
		for range time.Tick(pollInterval) {
			if time.Since(lastRun) > (recalculateInterval) {
				lastRun = ls.runAsyncdWorkers()
			}
		}
	}()

	l, err := ls.startListener()
	if err != nil {
		return fmt.Errorf("starting listener: %w", err)
	}

	if ls.tlsCerts != nil && len(ls.tlsCerts) > 0 {
		level.Debug(ls.logger).Log("message", "Using TLS")

		tlsConfig := &tls.Config{Certificates: ls.tlsCerts, InsecureSkipVerify: true} // lgtm[go/disabled-certificate-check]

		l = tls.NewListener(l, tlsConfig)
	} else {
		level.Debug(ls.logger).Log("message", "No TLS")
	}

	return ls.srv.Serve(l)
}

func (ls *localServer) Stop() error {
	level.Debug(ls.logger).Log("msg", "Stopping")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := ls.srv.Shutdown(ctx); err != nil {
		level.Info(ls.logger).Log("message", "got error shutting down", "error", err)
	}

	// Consider calling srv.Stop as a more forceful shutdown?

	return nil
}

func (ls *localServer) Interrupt(err error) {
	level.Debug(ls.logger).Log("message", "Stopping due to interrupt", "reason", err)
	if err := ls.Stop(); err != nil {
		level.Info(ls.logger).Log("message", "got error interrupting", "error", err)
	}
}

func (ls *localServer) startListener() (net.Listener, error) {
	for _, p := range portList {
		level.Debug(ls.logger).Log("msg", "Trying port", "port", p)

		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			level.Debug(ls.logger).Log(
				"message", "Unable to bind to port. Moving on",
				"port", p,
				"err", err,
			)
			continue
		}

		level.Info(ls.logger).Log("msg", "Got port", "port", p)
		return l, nil
	}

	return nil, errors.New("unable to bind to a local port")
}

func (ls *localServer) preflightCorsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Think harder, maybe?
		// https://stackoverflow.com/questions/12830095/setting-http-headers
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers",
			"Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		// Stop here if its Preflighted OPTIONS request
		if r.Method == "OPTIONS" {
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ls *localServer) rateLimitHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ls.limiter.Allow() == false {
			http.Error(w, http.StatusText(429), http.StatusTooManyRequests)
			level.Error(ls.logger).Log("msg", "Over rate limit")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ls *localServer) kryptoBoxOutboundHandler(http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	})
}

func (ls *localServer) verify(message []byte, sig []byte) error {
	return krypto.RsaVerify(ls.serverKey, message, sig)
}
