package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/kolide/kit/env"
	osquerygo "github.com/osquery/osquery-go"
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
			return fmt.Errorf("reading supplied queries file: %w", err)
		}
		if err := json.Unmarshal(data, &queries); err != nil {
			return fmt.Errorf("unmarshalling queries file json: %w", err)
		}
	}

	if *flQueries == "" {
		stdinQueries, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		if err := json.Unmarshal(stdinQueries, &queries); err != nil {
			return fmt.Errorf("unmarshalling stdin queries json: %w", err)
		}
	}

	if *flSocket == "" {
		return errors.New("--socket must be defined")
	}

	client, err := osquerygo.NewClient(*flSocket, 5*time.Second)
	if err != nil {
		return fmt.Errorf("opening osquery client connection on "+*flSocket+": %w", err)
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
			return fmt.Errorf("running query: %w", err)
		}

		if resp.Status.Code != int32(0) {
			fmt.Println("Error running query:", resp.Status.Message)
		}

		results.Results[name] = resp.Response
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	if err := enc.Encode(results); err != nil {
		return fmt.Errorf("encoding JSON query results: %w", err)
	}

	return nil
}
