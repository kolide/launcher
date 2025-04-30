package localserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolide/launcher/ee/observability"
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

	// We only allow acceleration via this endpoint if this testing flag is set.
	if ls.knapsack.AllowOverlyBroadDt4aAcceleration() {
		ls.accelerate(r.Context())
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
