package debug

import (
	"context"
	"fmt"
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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
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
				level.Info(logger).Log(
					"msg", "starting debug server",
					"err", err,
				)
				continue
			}

			// Stop server on next signal
			<-sig
			if err := serv.Shutdown(context.Background()); err != nil {
				level.Info(logger).Log(
					"msg", "error shutting down debug server",
					"err", err,
				)
				continue
			}

			level.Info(logger).Log(
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
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, errors.Wrap(err, "opening socket")
	}

	go func() {
		if err := serv.Serve(listener); err != nil && err != http.ErrServerClosed {
			level.Info(logger).Log("msg", "debug server failed", "err", err)
		}
	}()

	addr := fmt.Sprintf("http://%s/debug/?token=%s", listener.Addr().String(), token.String())
	// Write the address to a file for easy access by users
	if err := ioutil.WriteFile(addrPath, []byte(addr), 0600); err != nil {
		return nil, errors.Wrap(err, "writing debug address")
	}

	level.Info(logger).Log(
		"msg", "debug server started",
		"addr", addr,
	)

	return &serv, nil
}

// The below handler code is adapted from MIT licensed github.com/e-dard/netbug
func handler(token string, logger log.Logger) http.Handler {
	info := struct {
		Profiles []*pprof.Profile
		Token    string
	}{
		Profiles: pprof.Profiles(),
		Token:    url.QueryEscape(token),
	}

	h := func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		switch name {
		case "":
			// Index page.
			if err := indexTmpl.Execute(w, info); err != nil {
				level.Info(logger).Log(
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
	return http.HandlerFunc(h)
}

func authHandler(token string, logger log.Logger) http.Handler {
	h := handler(token, logger)
	ah := func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("token") == token {
			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "Unauthorized.")
		}
	}
	return http.HandlerFunc(ah)
}

func registerAuthHandler(token string, mux *http.ServeMux, logger log.Logger) {
	mux.Handle(debugPrefix, http.StripPrefix(debugPrefix, authHandler(token, logger)))
}

var indexTmpl = template.Must(template.New("index").Parse(`<html>
  <head>
    <title>Debug Information</title>
  </head>
  <br>
  <body>
    profiles:<br>
    <table>
    {{range .Profiles}}
      <tr><td align=right>{{.Count}}<td><a href="{{.Name}}?debug=1&token={{$.Token}}">{{.Name}}</a>
    {{end}}
    <tr><td align=right><td><a href="profile?token={{.Token}}">CPU</a>
    <tr><td align=right><td><a href="trace?seconds=5&token={{.Token}}">5-second trace</a>
    <tr><td align=right><td><a href="trace?seconds=30&token={{.Token}}">30-second trace</a>
    </table>
    <br>
    debug information:<br>
    <table>
      <tr><td align=right><td><a href="cmdline?token={{.Token}}">cmdline</a>
      <tr><td align=right><td><a href="symbol?token={{.Token}}">symbol</a>
    <tr><td align=right><td><a href="goroutine?debug=2&token={{.Token}}">full goroutine stack dump</a><br>
    <table>
  </body>
</html>`))
