package debug

import (
	"context"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	nhpprof "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/kolide/launcher/log"
	"github.com/pkg/errors"
)

const debugSignal = syscall.SIGUSR1
const debugPrefix = "/debug/"

// AttachDebugHandler will attach a signal handler that will toggle the debug
// server state when SIGUSR1 is sent to the process.
func AttachDebugHandler(addrPath string, logger log.Logger) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, debugSignal)
	go func() {
		for {
			// Start server on first signal
			<-sig
			serv, err := startDebugServer(addrPath, logger)
			if err != nil {
				logger.Info(
					"msg", "starting debug server",
					"err", err,
				)
				continue
			}

			// Stop server on next signal
			<-sig
			if err := serv.Shutdown(context.Background()); err != nil {
				logger.Info(
					"msg", "error shutting down debug server",
					"err", err,
				)
				continue
			}

			logger.Info(
				"msg", "shutdown debug server",
			)
		}
	}()
}

func startDebugServer(addrPath string, logger log.Logger) (*http.Server, error) {
	// Generate new (random) token to use for debug server auth
	token, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrap(err, "generating debug token")
	}

	// Start the debug server
	r := http.NewServeMux()
	registerAuthHandler(token.String(), r, logger)
	serv := http.Server{
		Handler: r,
	}
	// Allow the OS to pick an open port. Not intended to be a security
	// mechanism, only intended to ensure we don't try to bind to an
	// already used port.
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, errors.Wrap(err, "opening socket")
	}

	go func() {
		if err := serv.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Info("msg", "debug server failed", "err", err)
		}
	}()

	url := url.URL{
		Scheme:   "http",
		Host:     listener.Addr().String(),
		Path:     "/debug/",
		RawQuery: "token=" + token.String(),
	}
	addr := url.String()
	// Write the address to a file for easy access by users
	if err := ioutil.WriteFile(addrPath, []byte(addr), 0600); err != nil {
		return nil, errors.Wrap(err, "writing debug address")
	}

	logger.Info(
		"msg", "debug server started",
		"addr", addr,
	)

	return &serv, nil
}

// The below handler code is adapted from MIT licensed github.com/e-dard/netbug
func handler(token string, logger log.Logger) http.HandlerFunc {
	info := struct {
		Profiles []*pprof.Profile
		Token    string
	}{
		Profiles: pprof.Profiles(),
		Token:    url.QueryEscape(token),
	}

	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		switch name {
		case "":
			// Index page.
			if err := indexTmpl.Execute(w, info); err != nil {
				logger.Info(
					"msg", "error rendering debug template",
					"err", err,
				)
				return
			}
		case "cmdline":
			nhpprof.Cmdline(w, r)
		case "profile":
			nhpprof.Profile(w, r)
		case "trace":
			nhpprof.Trace(w, r)
		case "symbol":
			nhpprof.Symbol(w, r)
		default:
			// Provides access to all profiles under runtime/pprof
			nhpprof.Handler(name).ServeHTTP(w, r)
		}
	}
}

func authHandler(token string, logger log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("token") == token {
			handler(token, logger).ServeHTTP(w, r)
		} else {
			http.Error(w, "Request must include valid token.", http.StatusUnauthorized)
		}
	}
}

func registerAuthHandler(token string, mux *http.ServeMux, logger log.Logger) {
	mux.Handle(debugPrefix, http.StripPrefix(debugPrefix, http.HandlerFunc(authHandler(token, logger))))
}

var indexTmpl = template.Must(template.New("index").Parse(`<html>
  <head>
    <title>Debug Information</title>
  </head>
  <br>
  <body>
    Profiles:<br>
    <table>
    {{range .Profiles}}
      <tr><td align=right>{{.Count}}<td><a href="{{.Name}}?debug=1&token={{$.Token}}">{{.Name}}</a>
    {{end}}
    <tr><td align=right><td><a href="profile?token={{.Token}}">CPU</a>
    <tr><td align=right><td><a href="trace?seconds=5&token={{.Token}}">5-second trace</a>
    <tr><td align=right><td><a href="trace?seconds=30&token={{.Token}}">30-second trace</a>
    </table>
    <br>
    Debug information:<br>
    <table>
      <tr><td align=right><td><a href="cmdline?token={{.Token}}">cmdline</a>
      <tr><td align=right><td><a href="symbol?token={{.Token}}">symbol</a>
    <tr><td align=right><td><a href="goroutine?debug=2&token={{.Token}}">full goroutine stack dump</a><br>
    <table>
  </body>
</html>`))
