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
	localserverDt4aInfoKey = []byte("localserver_zta_info")

	// allowlistedDt4aOriginsLookup contains the complete list of origins that are permitted to access the /dt4a endpoint.
	allowlistedDt4aOriginsLookup = map[string]struct{}{
		// Release extension
		"chrome-extension://gejiddohjgogedgjnonbofjigllpkmbf":  {},
		"chrome-extension://khgocmkkpikpnmmkgmdnfckapcdkgfaf":  {},
		"chrome-extension://aeblfdkhhhdcdjpifhhbdiojplfjncoa":  {},
		"chrome-extension://dppgmdbiimibapkepcbdbmkaabgiofem":  {},
		"moz-extension://dfbae458-fb6f-4614-856e-094108a80852": {},
		"moz-extension://25fc87fa-4d31-4fee-b5c1-c32a7844c063": {},
		"moz-extension://d634138d-c276-4fc8-924b-40a0ea21d284": {},
		// Development and internal builds
		"chrome-extension://hjlinigoblmkhjejkmbegnoaljkphmgo":  {},
		"moz-extension://0a75d802-9aed-41e7-8daa-24c067386e82": {},
		"chrome-extension://hiajhnnfoihkhlmfejoljaokdpgboiea":  {},
		"chrome-extension://kioanpobaefjdloichnjebbdafiloboa":  {},
		"chrome-extension://bkpbhnjcbehoklfkljkkbbmipaphipgl":  {},
	}
)

const (
	safariWebExtensionScheme = "safari-web-extension://"

	accelerateInterval = 5 * time.Second
	accelerateDuration = 5 * time.Minute
)

func (ls *localServer) requestDt4aInfoHandler() http.Handler {
	return http.HandlerFunc(ls.requestDt4aInfoHandlerFunc)
}

func (ls *localServer) requestDt4aInfoHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	// Validate origin. We expect to either have the origin set to an allowlisted value, or to be
	// present but empty, or to be missing. We will not allow a request with a nonempty origin
	// that is not in the allowlist.
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

	dt4aInfo, err := ls.knapsack.Dt4aInfoStore().Get(localserverDt4aInfoKey)
	if err != nil {
		traces.SetError(span, err)
		ls.slogger.Log(r.Context(), slog.LevelError,
			"could not retrieve dt4a info from store",
			"err", err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// No data stored yet
	if len(dt4aInfo) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(dt4aInfo)
}

func (ls *localServer) requestDt4aAccelerationHandler() http.Handler {
	return http.HandlerFunc(ls.requestDt4aAccelerationHandlerFunc)
}

func (ls *localServer) requestDt4aAccelerationHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	// Validate origin. We expect to either have the origin set to an allowlisted value, or to be
	// present but empty, or to be missing. We will not allow a request with a nonempty origin
	// that is not in the allowlist.
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
