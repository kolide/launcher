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

//go:embed ltest202208.kolideint.net.crt
var tlsCert []byte

//go:embed ltest202208.kolideint.net.key
var tlsKey []byte

type localServer struct {
	logger      log.Logger
	srv         http.Server
	identifiers identifiers
	limiter     *rate.Limiter
	tlsEnabled  bool

	myKey     *rsa.PrivateKey
	serverKey *rsa.PublicKey
}

const (
	defaultRateLimit = 5
	defaultRateBurst = 10
)

func New(logger log.Logger, db *bbolt.DB, tlsEnabled bool) (*localServer, error) {
	ls := &localServer{
		logger:     log.With(logger, "component", "localserver"),
		limiter:    rate.NewLimiter(defaultRateLimit, defaultRateBurst),
		tlsEnabled: tlsEnabled,
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

	mux.Handle("/in", kbrw.Unwrap(http.HandlerFunc(pongHandler)))

	srv := http.Server{
		Handler:           ls.whompCors(ls.rateLimitHandler(mux)),
		ReadTimeout:       500 * time.Millisecond,
		ReadHeaderTimeout: 50 * time.Millisecond,
		WriteTimeout:      50 * time.Millisecond,
		MaxHeaderBytes:    1024,
	}

	ls.srv = srv

	return ls, nil
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

func (ls *localServer) Start() error {
	l, err := ls.startListener()
	if err != nil {
		return fmt.Errorf("starting listener: %w", err)
	}

	// Test TLS
	if ls.tlsEnabled {
		level.Info(ls.logger).Log("message", "Using TLS")

		cert, err := tls.X509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return fmt.Errorf("failed to read test tls certs: %w", err)
		}

		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}

		l = tls.NewListener(l, tlsConfig)
	} else {
		level.Info(ls.logger).Log("message", "No TLS")
	}

	return ls.srv.Serve(l)
}

func (ls *localServer) Stop() error {
	level.Debug(ls.logger).Log("msg", "Stopping")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(100*time.Millisecond))
	defer cancel()

	if err := ls.srv.Shutdown(ctx); err != nil {
		level.Info(ls.logger).Log("message", "got error shutting down", "error", err)
	}

	// Consider calling srv.Stop as a more forceful shutdown?

	return nil
}

func (ls *localServer) Interrupt(err error) {
	level.Debug(ls.logger).Log("message", "Stopping due to interrupt", "reason", err)
	_ = ls.Stop()
}

func (ls *localServer) startListener() (net.Listener, error) {
	for _, p := range portList {
		level.Debug(ls.logger).Log("msg", "Trying port", "port", p)

		l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", p))
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

func (ls *localServer) whompCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		fmt.Println("Whomp cors")

		/*
								testy?

			fetch("http://localhost:10356/id")
			  .then(response => response.blob())
			  .then(blob=> console.log(blob));

		*/
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
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ls *localServer) kryptoBoxOutboundHandler(http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	})
}
