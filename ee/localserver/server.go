package localserver

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto"
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

const defaultServerKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAkeNJgRkJOow7LovGmrlW
1UzHkifTKQV1/8kX+p2MPLptGgPKlqpLnhZsGOhpHpswlUalgSZPyhBfM9Btdmps
QZ2PkZkgEiy62PleVSBeBtpGcwHibHTGamzmKVrji9GudAvU+qapfPGnr//275/1
E+mTriB5XBrHic11YmtCG6yg0Vw383n428pNF8QD/Bx8pzgkie2xKi/cHkc9B0S2
B2rdYyWP17o+blgEM+EgjukLouX6VYkbMYhkDcy6bcUYfknII/T84kuChHkuWyO5
msGeD7hPhtdB/h0O8eBWIiOQ6fH7exl71UfGTR6pYQmJMK1ZZeT7FeWVSGkswxkV
4QIDAQAB
-----END PUBLIC KEY-----
`

type Querier interface {
	Query(query string) ([]map[string]string, error)
}

type localServer struct {
	logger      log.Logger
	srv         *http.Server
	identifiers identifiers
	limiter     *rate.Limiter
	tlsCerts    []tls.Certificate
	querier     Querier

	myKey     *rsa.PrivateKey
	serverKey *rsa.PublicKey
}

const (
	defaultRateLimit = 5
	defaultRateBurst = 10
)

func New(logger log.Logger, db *bbolt.DB) (*localServer, error) {
	ls := &localServer{
		logger:  log.With(logger, "component", "localserver"),
		limiter: rate.NewLimiter(defaultRateLimit, defaultRateBurst),
	}

	// TODO: As there may be things that adjust the keys during runtime, we need to persist that across
	// restarts. We should load-old-state here. This is still pretty TBD, so don't angst too much.
	if err := ls.LoadDefaultKeyIfNotSet(); err != nil {
		return nil, err
	}

	// Consider polling this on an interval, so we get updates.
	privateKey, err := osquery.PrivateKeyFromDB(db)
	if err != nil {
		return nil, fmt.Errorf("fetching private key: %w", err)
	}
	ls.myKey = privateKey

	kbrw := &kryptoBoxResponseWriter{
		boxer: krypto.NewBoxer(ls.myKey, ls.serverKey),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", http.NotFound)
	mux.HandleFunc("/ping", pongHandler)
	mux.Handle("/id", kbrw.Wrap(ls.requestIdHandler()))
	mux.Handle("/id.png", kbrw.WrapPng(ls.requestIdHandler()))

	// Still testing this
	//mux.Handle("/in", kbrw.Unwrap(http.HandlerFunc(pongHandler)))

	srv := &http.Server{
		Handler:           ls.requestLoggingHandler(ls.preflightCorsHandler(ls.rateLimitHandler(mux))),
		ReadTimeout:       500 * time.Millisecond,
		ReadHeaderTimeout: 50 * time.Millisecond,
		WriteTimeout:      50 * time.Millisecond,
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

	serverKeyRaw, err := krypto.KeyFromPem([]byte(defaultServerKey))
	if err != nil {
		return fmt.Errorf("parsing default public key: %w", err)
	}

	serverKey, ok := serverKeyRaw.(*rsa.PublicKey)
	if !ok {
		return errors.New("Public key not an rsa public key")
	}

	ls.serverKey = serverKey

	return nil
}

func (ls *localServer) runAsyncdWorkers() time.Time {
	if err := ls.updateIdFields(); err != nil {
		level.Info(ls.logger).Log(
			"msg", "Got error updating id fields",
			"err", err,
		)
	}

	return time.Now()
}

func (ls *localServer) Start() error {
	// Spawn background workers
	go func() {
		lastRun := ls.runAsyncdWorkers()

		// to account for laptop sleep, we check each hour to
		// see if we've elapsed enough wall clock time
		for range time.Tick(time.Hour * 1) {
			if time.Now().Sub(lastRun) > (24 * time.Hour) {
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

	return nil, errors.New("Unable to bind to a local port")
}

func pongHandler(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json")

	data := []byte(`{"ping": "Kolide"}` + "\n")
	res.Write(data)

}

func (ls *localServer) preflightCorsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Think harder, maybe?
		// https://stackoverflow.com/questions/12830095/setting-http-headers
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			fmt.Println("origin:, origin")
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
