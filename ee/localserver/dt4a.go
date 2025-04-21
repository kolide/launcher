package localserver

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/traces"
)

var (
	// legacyDt4aInfoKey is the key that was used to store dt4a info that was not tied to dt4a IDs
	// can be removed when unauthed /zta endpoint is removed
	legacyDt4aInfoKey = []byte("localserver_zta_info")
)

const (
	accelerateInterval = 5 * time.Second
	accelerateDuration = 5 * time.Minute
)

func (ls *localServer) requestDt4aInfoHandler() http.Handler {
	return http.HandlerFunc(ls.requestDt4aInfoHandlerFunc)
}

func (ls *localServer) requestDt4aInfoHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	// This check is superfluous with the check in the middleware -- but we still have one
	// unauthenticated endpoint that points directly to this handler, so we're leaving the check
	// in both places for now. We can remove it once /zta is removed.
	requestOrigin := r.Header.Get("Origin")
	if requestOrigin != "" {
		if _, ok := allowlistedDt4aOriginsLookup[requestOrigin]; !ok && !strings.HasPrefix(requestOrigin, safariWebExtensionScheme) {
			escapedOrigin := strings.ReplaceAll(strings.ReplaceAll(requestOrigin, "\n", ""), "\r", "") // remove any newlines
			ls.slogger.Log(r.Context(), slog.LevelInfo,
				"received dt4a request with origin not in allowlist",
				"req_origin", escapedOrigin,
			)
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	// We only allow acceleration via this endpoint if this testing flag is set.
	if ls.knapsack.AllowOverlyBroadDt4aAcceleration() {
		ls.accelerate(r.Context())
	}

	if r.Header.Get(dt4aAccountUuidHeaderKey) != "" {
		dt4aInfo, err := ls.knapsack.Dt4aInfoStore().Get([]byte(r.Header.Get(dt4aAccountUuidHeaderKey)))
		if err != nil {
			ls.slogger.Log(r.Context(), slog.LevelWarn,
				"could not retrieve dt4a info from store using dt4a account uuid",
				"err", err,
			)
		}

		if len(dt4aInfo) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Write(dt4aInfo)
			return
		}
	}

	// did not get info using account uuid, so we will try to get it using the legacy key
	info, err := ls.knapsack.Dt4aInfoStore().Get(legacyDt4aInfoKey)
	if err != nil {
		ls.slogger.Log(r.Context(), slog.LevelWarn,
			"could not retrieve dt4a info from store using legacy dt4a key",
			"err", err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(info) <= 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(info)
}

func (ls *localServer) requestDt4aAccelerationHandler() http.Handler {
	return http.HandlerFunc(ls.requestDt4aAccelerationHandlerFunc)
}

func (ls *localServer) requestDt4aAccelerationHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	ls.accelerate(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

func (ls *localServer) accelerate(ctx context.Context) {
	// Accelerate requests to control server
	ls.knapsack.SetControlRequestIntervalOverride(accelerateInterval, accelerateDuration)

	// Accelerate osquery distributed requests
	ls.knapsack.SetDistributedForwardingIntervalOverride(accelerateInterval, accelerateDuration)

	ls.slogger.Log(ctx, slog.LevelInfo,
		"accelerated control server and osquery distributed requests",
		"interval", accelerateInterval.String(),
		"duration", accelerateDuration.String(),
	)
}
