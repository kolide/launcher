//go:build !windows
// +build !windows

package debug

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	nhpprof "net/http/pprof"
	"net/url"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/google/uuid"
	"github.com/kolide/launcher/ee/gowrapper"
)

const debugPrefix = "/debug/"

func startDebugServer(addrPath string, slogger *slog.Logger) (*http.Server, error) {
	// Generate new (random) token to use for debug server auth
	token, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("generating debug token: %w", err)
	}

	// Start the debug server
	r := http.NewServeMux()
	registerAuthHandler(token.String(), r, slogger)
	serv := http.Server{
		Handler: r,
	}
	// Allow the OS to pick an open port. Not intended to be a security
	// mechanism, only intended to ensure we don't try to bind to an
	// already used port.
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("opening socket: %w", err)
	}

	gowrapper.Go(context.TODO(), slogger, func() {
		if err := serv.Serve(listener); err != nil && err != http.ErrServerClosed {
			slogger.Log(context.TODO(), slog.LevelInfo,
				"debug server failed",
				"err", err,
			)
		}
	})

	url := url.URL{
		Scheme:   "http",
		Host:     listener.Addr().String(),
		Path:     "/debug/",
		RawQuery: "token=" + token.String(),
	}
	addr := url.String()
	// Write the address to a file for easy access by users
	if err := os.WriteFile(addrPath, []byte(addr), 0600); err != nil {
		return nil, fmt.Errorf("writing debug address: %w", err)
	}

	slogger.Log(context.TODO(), slog.LevelInfo,
		"debug server started",
		"addr", addr,
	)

	return &serv, nil
}

// The below handler code is adapted from MIT licensed github.com/e-dard/netbug
func handler(token string, slogger *slog.Logger) http.HandlerFunc {
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
				slogger.Log(r.Context(), slog.LevelInfo,
					"error rendering debug template",
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

func authHandler(token string, slogger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("token") == token {
			handler(token, slogger).ServeHTTP(w, r)
		} else {
			http.Error(w, "Request must include valid token.", http.StatusUnauthorized)
		}
	}
}

func registerAuthHandler(token string, mux *http.ServeMux, slogger *slog.Logger) {
	mux.Handle(debugPrefix, http.StripPrefix(debugPrefix, authHandler(token, slogger)))
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
