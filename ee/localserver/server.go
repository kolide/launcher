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
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	knapsack     types.Knapsack
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

type LocalServerOption func(*localServer)

func WithLogger(logger log.Logger) LocalServerOption {
	return func(s *localServer) {
		s.logger = log.With(logger, "component", "localserver")
	}
}

func New(k types.Knapsack, opts ...LocalServerOption) (*localServer, error) {
	ls := &localServer{
		logger:                log.NewNopLogger(),
		knapsack:              k,
		limiter:               rate.NewLimiter(defaultRateLimit, defaultRateBurst),
		kolideServer:          k.KolideServerURL(),
		myLocalDbSigner:       agent.LocalDbKeys(),
		myLocalHardwareSigner: agent.HardwareKeys(),
	}

	for _, o := range opts {
		o(ls)
	}

	// TODO: As there may be things that adjust the keys during runtime, we need to persist that across
	// restarts. We should load-old-state here. This is still pretty TBD, so don't angst too much.
	if err := ls.LoadDefaultKeyIfNotSet(); err != nil {
		return nil, err
	}

	// Consider polling this on an interval, so we get updates.
	privateKey, err := osquery.PrivateRSAKeyFromDB(k.ConfigStore())
	if err != nil {
		return nil, fmt.Errorf("fetching private key: %w", err)
	}
	ls.myKey = privateKey

	ecKryptoMiddleware := newKryptoEcMiddleware(ls.logger, ls.myLocalDbSigner, ls.myLocalHardwareSigner, *ls.serverEcKey)
	ecAuthedMux := http.NewServeMux()
	ecAuthedMux.HandleFunc("/", http.NotFound)
	ecAuthedMux.Handle("/acceleratecontrol", ls.requestAccelerateControlHandler())
	ecAuthedMux.Handle("/acceleratecontrol.png", ls.requestAccelerateControlHandler())
	ecAuthedMux.Handle("/id", ls.requestIdHandler())
	ecAuthedMux.Handle("/id.png", ls.requestIdHandler())
	ecAuthedMux.Handle("/query", ls.requestQueryHandler())
	ecAuthedMux.Handle("/query.png", ls.requestQueryHandler())
	ecAuthedMux.Handle("/scheduledquery", ls.requestScheduledQueryHandler())
	ecAuthedMux.Handle("/scheduledquery.png", ls.requestScheduledQueryHandler())

	mux := http.NewServeMux()
	mux.HandleFunc("/", http.NotFound)
	mux.Handle("/v0/cmd", ecKryptoMiddleware.Wrap(ecAuthedMux))

	// /v1/cmd was added after fixing a bug where local server would panic when an endpoint was not found
	// after making it through the kryptoEcMiddleware
	// by using v1, k2 can call endpoints without fear of panicing local server
	// /v0/cmd left for transition period
	mux.Handle("/v1/cmd", ecKryptoMiddleware.Wrap(ecAuthedMux))

	// uncomment to test without going through middleware
	// for example:
	// curl localhost:40978/query --data '{"query":"select * from kolide_launcher_info"}'
	// mux.Handle("/query", ls.requestQueryHandler())
	// curl localhost:40978/scheduledquery --data '{"name":"pack:kolide_device_updaters:agentprocesses-all:snapshot"}'
	// mux.Handle("/scheduledquery", ls.requestScheduledQueryHandler())
	// curl localhost:40978/acceleratecontrol  --data '{"interval":"250ms", "duration":"1s"}'
	// mux.Handle("/acceleratecontrol", ls.requestAccelerateControlHandler())

	srv := &http.Server{
		Handler: otelhttp.NewHandler(ls.requestLoggingHandler(ls.preflightCorsHandler(ls.rateLimitHandler(mux))), "localserver", otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.URL.Path
		})),
		ReadTimeout:       500 * time.Millisecond,
		ReadHeaderTimeout: 50 * time.Millisecond,
		// WriteTimeout very high due to retry logic in the scheduledquery endpoint
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1024,
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
	case strings.HasPrefix(ls.kolideServer, "localhost"), strings.HasPrefix(ls.kolideServer, "127.0.0.1"), strings.Contains(ls.kolideServer, ".ngrok."):
		level.Debug(ls.logger).Log("msg", "using developer certificates")
		serverRsaCertPem = localhostRsaServerCert
		serverEccCertPem = localhostEccServerCert
	case strings.HasSuffix(ls.kolideServer, ".herokuapp.com"):
		level.Debug(ls.logger).Log("msg", "using review app certificates")
		serverRsaCertPem = reviewRsaServerCert
		serverEccCertPem = reviewEccServerCert
	default:
		level.Debug(ls.logger).Log("msg", "using default/production certificates")
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
	// Spawn background workers. The information gathered here is not critical for DT flow- so to reduce early osquery contention
	// we wait for <pollInterval> and before starting and then only rerun if the previous run was unsuccessful,
	// or has been greater than <recalculateInterval>. Note that this polling is merely a check against time,
	// we don't repopulate this data nearly so often. (But we poll frequently to account for the difference between
	// wall clock time, and sleep time)
	const (
		pollInterval        = 15 * time.Minute
		recalculateInterval = 24 * time.Hour
	)

	go func() {
		var lastRun time.Time

		// note that this will trigger the check for the first time after pollInterval (not immediately)
		for range time.Tick(pollInterval) {
			if time.Since(lastRun) > recalculateInterval {
				lastRun = ls.runAsyncdWorkers()
				if lastRun.IsZero() {
					level.Debug(ls.logger).Log("message", "runAsyncdWorkers unsuccessful, will retry in the future.")
				}
			}
		}
	}()

	l, err := ls.startListener()
	if err != nil {
		return fmt.Errorf("starting listener: %w", err)
	}

	if ls.tlsCerts != nil && len(ls.tlsCerts) > 0 {
		level.Debug(ls.logger).Log("message", "Using TLS")

		tlsConfig := &tls.Config{Certificates: ls.tlsCerts}

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

func (ls *localServer) Interrupt(_ error) {
	level.Debug(ls.logger).Log("message", "Stopping due to interrupt")
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

		// Some modern chrome and derivatives use Access-Control-Allow-Private-Network
		// https://developer.chrome.com/blog/private-network-access-preflight/
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

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
