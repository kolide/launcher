package localserver

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/observability"
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
	r, span := observability.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	// This check is superfluous with the check in the middleware -- but we still have one
	// unauthenticated endpoint that points directly to this handler, so we're leaving the check
	// in both places for now. We can remove it once /zta is removed.
	requestOrigin := r.Header.Get("Origin")
	if !originIsAllowlisted(requestOrigin) {
		escapedOrigin := strings.ReplaceAll(strings.ReplaceAll(requestOrigin, "\n", ""), "\r", "") // remove any newlines
		ls.slogger.Log(r.Context(), slog.LevelInfo,
			"received dt4a request with origin not in allowlist",
			"req_origin", escapedOrigin,
		)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// We only allow acceleration via this endpoint if this testing flag is set.
	if ls.knapsack.AllowOverlyBroadDt4aAcceleration() {
		ls.accelerate(r.Context())
	}

	// this should be removed when we drop unauthed endpoint
	if r.Header.Get(dt4aAccountUuidHeaderKey) == "" {
		// This is a legacy request to the unauthed endpoint that does not include the dt4a account uuid header.
		// We will return the dt4a info stored under the legacy key.
		dt4aInfo, err := ls.knapsack.Dt4aInfoStore().Get(legacyDt4aInfoKey)
		if err != nil {
			ls.slogger.Log(r.Context(), slog.LevelWarn,
				"could not retrieve dt4a info from store using legacy dt4a key",
				"err", err,
			)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if len(dt4aInfo) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(dt4aInfo)
		return
	}

	// dt4aAccountUuid is set, so we will try to get the dt4a info using the account uuid
	dt4aInfo, err := ls.knapsack.Dt4aInfoStore().Get([]byte(r.Header.Get(dt4aAccountUuidHeaderKey)))
	if err != nil {
		observability.SetError(span, err)
		ls.slogger.Log(r.Context(), slog.LevelError,
			"could not retrieve dt4a info from store using dt4a account uuid",
			"err", err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(dt4aInfo) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(dt4aInfo)
}

func (ls *localServer) requestDt4aAccelerationHandler() http.Handler {
	return http.HandlerFunc(ls.requestDt4aAccelerationHandlerFunc)
}

func (ls *localServer) requestDt4aAccelerationHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := observability.StartHttpRequestSpan(r, "path", r.URL.Path)
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

func (ls *localServer) requestDt4aHealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
}
