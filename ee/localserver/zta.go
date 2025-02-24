package localserver

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/kolide/launcher/pkg/traces"
)

var (
	localserverZtaInfoKey = []byte("localserver_zta_info")

	// allowlistedZtaOriginsLookup contains the complete list of origins that are permitted to access the /zta endpoint.
	allowlistedZtaOriginsLookup = map[string]struct{}{
		// Release extension
		"chrome-extension://gejiddohjgogedgjnonbofjigllpkmbf":         {},
		"chrome-extension://khgocmkkpikpnmmkgmdnfckapcdkgfaf":         {},
		"chrome-extension://aeblfdkhhhdcdjpifhhbdiojplfjncoa":         {},
		"chrome-extension://dppgmdbiimibapkepcbdbmkaabgiofem":         {},
		"moz-extension://dfbae458-fb6f-4614-856e-094108a80852":        {},
		"moz-extension://25fc87fa-4d31-4fee-b5c1-c32a7844c063":        {},
		"moz-extension://d634138d-c276-4fc8-924b-40a0ea21d284":        {},
		"safari-web-extension://7cba6ce0-bc55-43bd-b2a3-562b703ecbea": {},
		// Development and internal builds
		"chrome-extension://hjlinigoblmkhjejkmbegnoaljkphmgo":  {},
		"moz-extension://0a75d802-9aed-41e7-8daa-24c067386e82": {},
		"chrome-extension://hiajhnnfoihkhlmfejoljaokdpgboiea":  {},
		"chrome-extension://kioanpobaefjdloichnjebbdafiloboa":  {},
	}
)

func (ls *localServer) requestZtaInfoHandler() http.Handler {
	return http.HandlerFunc(ls.requestZtaInfoHandlerFunc)
}

func (ls *localServer) requestZtaInfoHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Validate origin
	requestOrigin := r.Header.Get("Origin")
	if _, ok := allowlistedZtaOriginsLookup[requestOrigin]; !ok {
		escapedOrigin := strings.ReplaceAll(strings.ReplaceAll(requestOrigin, "\n", ""), "\r", "") // remove any newlines
		ls.slogger.Log(r.Context(), slog.LevelInfo,
			"received zta request with origin not in allowlist",
			"req_origin", escapedOrigin,
		)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	ztaInfo, err := ls.knapsack.ZtaInfoStore().Get(localserverZtaInfoKey)
	if err != nil {
		traces.SetError(span, err)
		ls.slogger.Log(r.Context(), slog.LevelError,
			"could not retrieve ZTA info from store",
			"err", err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// No data stored yet
	if len(ztaInfo) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(ztaInfo)
}
