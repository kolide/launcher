package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/kolide/kit/env"
	osquerygo "github.com/kolide/osquery-go"
	"github.com/pkg/errors"
)

type queryFile struct {
	Queries map[string]string `json:"queries"`
}

func runQuery(args []string) error {
	flagset := flag.NewFlagSet("launcher query", flag.ExitOnError)
	var (
		flQueries = flagset.String(
			"queries",
			env.String("QUERIES", ""),
			"A file containing queries to run",
		)
		flSocket = flagset.String(
			"socket",
			env.String("SOCKET", ""),
			"The path to the socket",
		)
	)
	flagset.Usage = commandUsage(flagset, "launcher query")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	var queries queryFile

	if _, err := os.Stat(*flQueries); err == nil {
		data, err := ioutil.ReadFile(*flQueries)
		if err != nil {
			return errors.Wrap(err, "reading supplied queries file")
		}
		if err := json.Unmarshal(data, &queries); err != nil {
			return errors.Wrap(err, "unmarshalling queries file json")
		}
	}

	if *flQueries == "" {
		stdinQueries, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "reading stdin")
		}
		if err := json.Unmarshal(stdinQueries, &queries); err != nil {
			return errors.Wrap(err, "unmarshalling stdin queries json")
		}
	}

	if *flSocket == "" {
		return errors.New("--socket must be defined")
	}

	client, err := osquerygo.NewClient(*flSocket, 5*time.Second)
	if err != nil {
		return errors.Wrap(err, "opening osquery client connection on "+*flSocket)
	}
	defer client.Close()

	results := struct {
		Results map[string]interface{} `json:"results"`
	}{
		Results: map[string]interface{}{},
	}

	for name, query := range queries.Queries {
		resp, err := client.Query(query)
		if err != nil {
			return errors.Wrap(err, "running query")
		}

		if resp.Status.Code != int32(0) {
			fmt.Println("Error running query:", resp.Status.Message)
		}

		results.Results[name] = resp.Response
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	if err := enc.Encode(results); err != nil {
		return errors.Wrap(err, "encoding JSON query results")
	}

	return nil
}
