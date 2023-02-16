package localserver

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (ls *localServer) requestQueryHandler() http.Handler {
	return http.HandlerFunc(ls.requestQueryHanlderFunc)
}

func (ls *localServer) requestQueryHanlderFunc(res http.ResponseWriter, r *http.Request) {
	// now check body
	if r.Body == nil {
		res.Write([]byte("request body is nil"))
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		res.Write([]byte(fmt.Sprintf("error unmarshaling request body: %s", err)))
		return
	}

	query, ok := body["query"]
	if !ok {
		res.Write([]byte("no query key found in request body json"))
		return
	}

	if query == "" {
		res.Write([]byte("empty query"))
		return
	}

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
