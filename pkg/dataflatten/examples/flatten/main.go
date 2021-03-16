package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/peterbourgon/ff/v3"
	"github.com/pkg/errors"
)

func checkError(err error) {
	if err != nil {
		fmt.Printf("Got Error: %v\nStack:\n%+v\n", err, err)
		os.Exit(1)
	}
}

func main() {

	flagset := flag.NewFlagSet("plist", flag.ExitOnError)

	var (
		flPlist = flagset.String("plist", "", "Path to plist")
		flJson  = flagset.String("json", "", "Path to json file")
		flXml   = flagset.String("xml", "", "Path to xml file")
		flIni   = flagset.String("ini", "", "Path to ini file")
		flQuery = flagset.String("q", "", "query")

		flDebug = flagset.Bool("debug", false, "use a debug logger")
	)

	if err := ff.Parse(flagset, os.Args[1:],
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
	); err != nil {
		checkError(errors.Wrap(err, "parsing flags"))
	}

	logger := logutil.NewCLILogger(*flDebug)

	opts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(logger),
		dataflatten.WithNestedPlist(),
		dataflatten.WithQuery(strings.Split(*flQuery, `/`)),
	}

	rows := []dataflatten.Row{}

	if *flPlist != "" {
		data, err := dataflatten.PlistFile(*flPlist, opts...)
		checkError(errors.Wrap(err, "flattening plist file"))
		rows = append(rows, data...)
	}

	if *flJson != "" {
		data, err := dataflatten.JsonFile(*flJson, opts...)
		checkError(errors.Wrap(err, "flattening json file"))
		rows = append(rows, data...)
	}

	if *flXml != "" {
		data, err := dataflatten.XmlFile(*flXml, opts...)
		checkError(errors.Wrap(err, "flattening xml file"))
		rows = append(rows, data...)
	}

	if *flIni != "" {
		data, err := dataflatten.IniFile(*flIni, opts...)
		checkError(errors.Wrap(err, "flattening ini file"))
		rows = append(rows, data...)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", "path", "parent key", "key", "value")
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", "----", "----------", "---", "-----")

	for _, row := range rows {
		p, k := row.ParentKey("/")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", row.StringPath("/"), p, k, row.Value)
	}
	w.Flush()

	return
}
