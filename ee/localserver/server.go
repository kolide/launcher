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
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/krypto"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/time/rate"
)

// Special Kolide Ports
var PortList = []int{
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
	slogger      *slog.Logger
	knapsack     types.Knapsack
	srv          *http.Server
	identifiers  identifiers
	limiter      *rate.Limiter
	tlsCerts     []tls.Certificate
	querier      Querier
	kolideServer string
	cancel       context.CancelFunc

	myKey           *rsa.PrivateKey
	myLocalDbSigner crypto.Signer

	serverKey   *rsa.PublicKey
	serverEcKey *ecdsa.PublicKey

	tenantMunemo string
}

const (
	defaultRateLimit = 5
	defaultRateBurst = 10
)

type presenceDetector interface {
	DetectPresence(reason string, interval time.Duration) (time.Duration, error)
}

func New(ctx context.Context, k types.Knapsack, presenceDetector presenceDetector) (*localServer, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	ls := &localServer{
		slogger:         k.Slogger().With("component", "localserver"),
		knapsack:        k,
		limiter:         rate.NewLimiter(defaultRateLimit, defaultRateBurst),
		kolideServer:    k.KolideServerURL(),
		myLocalDbSigner: agent.LocalDbKeys(),
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

	ecKryptoMiddleware := newKryptoEcMiddleware(k.Slogger(), ls.myLocalDbSigner, *ls.serverEcKey, presenceDetector)
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
	mux.Handle("/v0/cmd", ecKryptoMiddleware.Wrap(ls.munemoCheckHandler(ecAuthedMux)))

	// /v1/cmd was added after fixing a bug where local server would panic when an endpoint was not found
	// after making it through the kryptoEcMiddleware
	// by using v1, k2 can call endpoints without fear of panicing local server
	// /v0/cmd left for transition period
	mux.Handle("/v1/cmd", ecKryptoMiddleware.Wrap(ls.munemoCheckHandler(ecAuthedMux)))

	// uncomment to test without going through middleware
	// for example:
	// curl localhost:40978/query --data '{"query":"select * from kolide_launcher_info"}'
	// mux.Handle("/query", ls.requestQueryHandler())
	// curl localhost:40978/scheduledquery --data '{"name":"pack:kolide_device_updaters:agentprocesses-all:snapshot"}'
	// mux.Handle("/scheduledquery", ls.requestScheduledQueryHandler())
	// curl localhost:40978/acceleratecontrol  --data '{"interval":"250ms", "duration":"1s"}'
	// mux.Handle("/acceleratecontrol", ls.requestAccelerateControlHandler())
	// curl localhost:40978/id
	// mux.Handle("/id", ls.requestIdHandler())

	srv := &http.Server{
		Handler: otelhttp.NewHandler(
			ls.requestLoggingHandler(
				ls.preflightCorsHandler(
					ls.rateLimitHandler(
						mux,
					),
				)), "localserver", otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
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

	ctx := context.TODO()
	slogLevel := slog.LevelDebug

	switch {
	case strings.HasPrefix(ls.kolideServer, "localhost"), strings.HasPrefix(ls.kolideServer, "127.0.0.1"), strings.Contains(ls.kolideServer, ".ngrok."):
		ls.slogger.Log(ctx, slogLevel,
			"using developer certificates",
		)

		serverRsaCertPem = localhostRsaServerCert
		serverEccCertPem = localhostEccServerCert
	case strings.HasSuffix(ls.kolideServer, ".herokuapp.com"):
		ls.slogger.Log(ctx, slogLevel,
			"using review app certificates",
		)

		serverRsaCertPem = reviewRsaServerCert
		serverEccCertPem = reviewEccServerCert
	default:
		ls.slogger.Log(ctx, slogLevel,
			"using default/production certificates",
		)
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

	ctx := context.TODO()
	ls.slogger.Log(ctx, slog.LevelDebug,
		"starting async worker run",
	)

	if err := ls.updateIdFields(); err != nil {
		success = false
		ls.slogger.Log(ctx, slog.LevelError,
			"updating id fields",
			"err", err,
		)
	}

	ls.slogger.Log(ctx, slog.LevelDebug,
		"completed async worker run",
		"success", success,
	)

	if !success {
		return time.Time{}
	}
	return time.Now()
}

var (
	pollInterval        = 15 * time.Minute
	recalculateInterval = 24 * time.Hour
)

func (ls *localServer) Start() error {
	// Spawn background workers. The information gathered here is not critical for DT flow- so to reduce early osquery contention
	// we wait for <pollInterval> and before starting and then only rerun if the previous run was unsuccessful,
	// or has been greater than <recalculateInterval>. Note that this polling is merely a check against time,
	// we don't repopulate this data nearly so often. (But we poll frequently to account for the difference between
	// wall clock time, and sleep time)

	var ctx context.Context
	ctx, ls.cancel = context.WithCancel(context.Background())

	gowrapper.Go(ctx, ls.slogger, func() {
		var lastRun time.Time

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		// note that this will trigger the check for the first time after pollInterval (not immediately)
		for {
			select {
			case <-ctx.Done():
				ls.slogger.Log(ctx, slog.LevelDebug,
					"runAsyncdWorkers received shutdown signal",
				)
				return
			case <-ticker.C:
				if time.Since(lastRun) > recalculateInterval {
					lastRun = ls.runAsyncdWorkers()
					if lastRun.IsZero() {
						ls.slogger.Log(ctx, slog.LevelDebug,
							"runAsyncdWorkers unsuccessful, will retry in the future",
						)
					}
				}
			}
		}
	})

	l, err := ls.startListener()
	if err != nil {
		return fmt.Errorf("starting listener: %w", err)
	}

	if len(ls.tlsCerts) > 0 {
		ls.slogger.Log(ctx, slog.LevelDebug,
			"using TLS",
		)

		tlsConfig := &tls.Config{Certificates: ls.tlsCerts}

		l = tls.NewListener(l, tlsConfig)
	} else {
		ls.slogger.Log(ctx, slog.LevelDebug,
			"not using TLS",
		)
	}

	return ls.srv.Serve(l)
}

func (ls *localServer) Stop() error {
	ctx := context.TODO()
	ls.slogger.Log(ctx, slog.LevelDebug,
		"stopping",
	)

	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	if err := ls.srv.Shutdown(ctx); err != nil {
		ls.slogger.Log(ctx, slog.LevelError,
			"shutting down",
			"err", err,
		)
	}

	// Consider calling srv.Stop as a more forceful shutdown?

	return nil
}

func (ls *localServer) Interrupt(_ error) {
	ctx := context.TODO()

	ls.slogger.Log(ctx, slog.LevelDebug,
		"stopping due to interrupt",
	)

	if err := ls.Stop(); err != nil {
		ls.slogger.Log(ctx, slog.LevelError,
			"stopping",
			"err", err,
		)
	}

	ls.cancel()
}

func (ls *localServer) startListener() (net.Listener, error) {
	ctx := context.TODO()

	for _, p := range PortList {
		ls.slogger.Log(ctx, slog.LevelDebug,
			"trying port",
			"port", p,
		)

		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			ls.slogger.Log(ctx, slog.LevelDebug,
				"unable to bind to port, moving on",
				"port", p,
				"err", err,
			)

			continue
		}

		ls.slogger.Log(ctx, slog.LevelInfo,
			"got port",
			"port", p,
		)
		return l, nil
	}

	return nil, errors.New("unable to bind to a local port")
}

func (ls *localServer) preflightCorsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We don't believe we can meaningfully enforce a CORS style check here -- those are enforced by the browser.
		// And we recognize there are some patterns that bypass the browsers CORS enforcement. However, we do implement
		// origin enforcement as an allowlist inside kryptoEcMiddleware
		// See https://github.com/kolide/k2/issues/9634
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers",
			"Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		// We need this for device trust to work in newer versions of Chrome with experimental features toggled on
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Some modern chrome and derivatives use Access-Control-Allow-Private-Network
		// https://developer.chrome.com/blog/private-network-access-preflight/
		// Though it's unclear if this is still needed, see https://developer.chrome.com/blog/private-network-access-update/
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
		if !ls.limiter.Allow() {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)

			ls.slogger.Log(r.Context(), slog.LevelError,
				"over rate limit",
			)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// munemoCheckHandler returns an http error if the request's X-Kolide-Munemo header does not match the organization in the enroll secret
// if either the header or the enroll secret are missing, the request is allowed through
func (ls *localServer) munemoCheckHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		munemoHeaderValue := r.Header.Get("X-Kolide-Munemo")
		if munemoHeaderValue == "" {
			ls.slogger.Log(r.Context(), slog.LevelDebug,
				"munemo header is empty, continuing",
			)

			next.ServeHTTP(w, r)
			return
		}

		localServerMunemo, err := ls.getMunemoFromEnrollSecret()
		if err != nil {
			ls.slogger.Log(r.Context(), slog.LevelError,
				"getting munemo from enroll secret, conitnuing",
				"err", err,
			)

			next.ServeHTTP(w, r)
			return
		}

		if localServerMunemo == "" {
			ls.slogger.Log(r.Context(), slog.LevelDebug,
				"munemo in enroll secret is empty, continuing",
				"header", munemoHeaderValue,
			)

			next.ServeHTTP(w, r)
			return
		}

		if munemoHeaderValue == localServerMunemo {
			next.ServeHTTP(w, r)
			return
		}

		ls.slogger.Log(r.Context(), slog.LevelDebug,
			"munemo in request does not match munemo in enroll secret",
			"header", munemoHeaderValue,
			"enroll_secret", localServerMunemo,
		)

		http.Error(w, "munemo in request does not match munemo in enroll secret", http.StatusUnauthorized)
		return
	})
}

// munemoCheckHandler returns an http error if the request's X-Kolide-Munemo header does not match the organization in the enroll secret
// if either the header or the enroll secret are missing, the request is allowed through
func (ls *localServer) getMunemoFromEnrollSecret() (string, error) {
	if ls.tenantMunemo != "" {
		return ls.tenantMunemo, nil
	}

	rawToken, err := ls.knapsack.ReadEnrollSecret()
	if err != nil {
		return "", err
	}

	// We do not have the key, and thus CANNOT verify. So this is ParseUnverified
	token, _, err := new(jwt.Parser).ParseUnverified(rawToken, jwt.MapClaims{})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	org, ok := claims["organization"]
	if !ok {
		return "", errors.New("no organization claim")
	}

	// convert org to string
	munemo, ok := org.(string)
	if !ok {
		return "", errors.New("organization claim not a string")
	}

	ls.tenantMunemo = munemo

	return munemo, nil
}
