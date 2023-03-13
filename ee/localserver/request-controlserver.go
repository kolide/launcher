package localserver

import (
	"fmt"
	"net/http"
)

func (ls *localServer) requestControlServerFetch() http.Handler {
	return http.HandlerFunc(ls.requestControlServerFetchFunc)
}

func (ls *localServer) requestControlServerFetchFunc(w http.ResponseWriter, r *http.Request) {
	if err := ls.controlServer.Fetch(); err != nil {
		sendClientError(w, fmt.Sprintf("error calling control server fetch: %s", err))
	}
}
