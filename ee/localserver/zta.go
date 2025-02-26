package localserver

import (
	"log/slog"
	"net/http"

	"github.com/kolide/launcher/pkg/traces"
)

var (
	localserverZtaInfoKey = []byte("localserver_zta_info")
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
