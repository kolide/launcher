package linux_important_updates_debian

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

var (
	pacManPath = []string{"/usr/bin/apt"}
	pacManCmds = []string{"list", "--upgradeable"}
	pacInstPath = []string{"/usr/bin/dpkg"}
	pacInstCmds = []string{"-p"}
)

type Table struct {
	client		*osquery.ExtensionManagerClient
	logger		log.Logger
	tableName	string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("package"),
		table.TextColumn("sources"),
		table.TextColumn("update_version"),
		table.TextColumn("current_version"),
		table.TextColumn("essential"),
		table.TextColumn("priority"),
		table.TextColumn("section"),
		table.TextColumn("task"),
		table.TextColumn("build_essential"),
		table.IntegerColumn("assumed_importance"),
	}

	t := &Table{
		client:    client,
		logger:    logger,
		tableName: "kolide_linux_important_updates_debian",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	//tablehelpers.Exec(ctx, t.logger, 20, pacManPath, []string{"update"}, false)

	pac_man_out, err := tablehelpers.Exec(ctx, t.logger, 5, pacManPath, pacManCmds, false)
	if err != nil {
		level.Info(t.logger).Log("msg", "apt list failed", "err", err)
	}
	pac_inst_out, err := tablehelpers.Exec(ctx, t.logger, 5, pacInstPath, pacInstCmds, false)
	if err != nil {
		level.Info(t.logger).Log("msg", "dpkg failed", "err", err)
	}

	aptout, err := aptOutput(pac_man_out)
	dpkgout, err := dpkgOutput(pac_inst_out)

	for dk, dv := range dpkgout {
		if _, ok := aptout[dk]; ok {
			for dvk, dvv := range dv {
				aptout[dk][dvk] = dvv
			}
		}
	}

	for  _, row := range aptout {
		importance := 0

		// Figuring out how to tell when an update *should*
		// be applied has been difficult. I've been trying
		// to keep system execs low, and for debian I hope
		// something along these lines works.
		//
		// Merging the upgradable apt sources with dpkg's
		// version info seems to give a good starting point.
		if strings.Contains(row["sources"], "-security") {
			importance = importance + 1
		}

		switch row["build_essential"] {
		case "yes":
			importance = importance + 1
		}

		switch row["essential"] {
		case "yes":
			importance = importance + 1
		}

		switch row["priority"] {
		case "important":
			importance = importance + 1
		case "required":
			importance = importance + 2
		}

		switch row["section"] {
		case "admin", "devel", "libs", "metapackages", "net", "shells", "utils":
			importance = importance + 1
		}

		/*
		switch row["task"] {
		default:
			importance = importance
		}
		*/

		row["assumed_importance"] = strconv.Itoa(importance)

		results = append(results, row)
	}

	return results, nil
}
