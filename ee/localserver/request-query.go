package localserver

import (
	"encoding/json"
	"net/http"
)

func (ls *localServer) requestQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestQueryHanlderFunc)
}

func (ls *localServer) requestQueryHanlderFunc(res http.ResponseWriter, req *http.Request) {
	query := req.URL.Query().Get("query")

	if query == "" {
		jsonBytes := []byte("no query parameter found in url parameters")
		res.Write(jsonBytes)
		return
	}

	results, err := ls.querier.Query(query)
	if err != nil {
		res.Write([]byte(err.Error()))
		return
	}

	jsonBytes, err := json.Marshal(results)
	if err != nil {
		res.Write([]byte(err.Error()))
	}

	res.Write(jsonBytes)
}
