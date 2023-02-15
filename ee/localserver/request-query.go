package localserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

func (ls *localServer) requestQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestQueryHanlderFunc)
}

func (ls *localServer) requestQueryHanlderFunc(res http.ResponseWriter, req *http.Request) {
	queryRaw := req.URL.Query().Get("query")
	if queryRaw == "" {
		res.Write([]byte("no query parameter found in url parameters"))
		return
	}

	queryDecoded, err := base64.StdEncoding.DecodeString(req.URL.Query().Get("query"))
	if err != nil {
		res.Write([]byte(fmt.Sprintf("error decoding query from b64: %s", err)))
		return
	}

	query := string(queryDecoded)

	results, err := ls.querier.Query(query)
	if err != nil {
		res.Write([]byte(fmt.Sprintf("error executing query: %s", err)))
		return
	}

	jsonBytes, err := json.Marshal(results)
	if err != nil {
		res.Write([]byte(fmt.Sprintf("error marshalling results to json: %s", err)))
		return
	}

	res.Write(jsonBytes)
}
