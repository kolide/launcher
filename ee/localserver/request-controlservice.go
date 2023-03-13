package localserver

import (
	"fmt"
	"net/http"
)

func (ls *localServer) requestControlServiceFetchHanlder() http.Handler {
	return http.HandlerFunc(ls.requestControlServiceFetchFunc)
}

func (ls *localServer) requestControlServiceFetchFunc(w http.ResponseWriter, r *http.Request) {
	if ls.controlService == nil {
		sendClientError(w, fmt.Sprintf("control service not configured"))
		return
	}

	if err := ls.controlService.Fetch(); err != nil {
		sendClientError(w, fmt.Sprintf("error calling control service fetch: %s", err))
	}
}
