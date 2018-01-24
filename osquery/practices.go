package osquery

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

type PracticeServer struct {
	results map[string]OsqueryQueryResults
}

func NewPracticeServer() *PracticeServer {
	return &PracticeServer{
		results: make(map[string]OsqueryQueryResults),
	}
}

func (p *PracticeServer) handler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(p.results)
}

func (p *PracticeServer) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/results", p.handler)
	http.ListenAndServe(addr, mux)
	return errors.New("http server ended")
}

func (p *PracticeServer) SetQueryResults(results map[string]OsqueryQueryResults) error {
	fmt.Println("results", results)
	p.results = results
	return nil
}
