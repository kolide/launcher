//go:build darwin
// +build darwin

package spotlight

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type spotlightTable struct {
	slogger *slog.Logger
}

/*
Spotlight returns a macOS spotlight table
Example Query:

	SELECT uid, f.path FROM file
	AS f JOIN kolide_spotlight ON spotlight.path = f.path
	AND spotlight.query = "kMDItemKint = 'Agile Keychain'";
*/
func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("query"),
		table.TextColumn("path"),
	}

	t := &spotlightTable{
		slogger: slogger.With("table", "kolide_spotlight"),
	}

	return tablewrapper.New(flags, slogger, "kolide_spotlight", columns, t.generate)
}

func (t *spotlightTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_spotlight")
	defer span.End()

	q, ok := queryContext.Constraints["query"]
	if !ok || len(q.Constraints) == 0 {
		return nil, errors.New("The spotlight table requires that you specify a constraint WHERE query =")
	}

	where := q.Constraints[0].Expression
	var query []string
	if strings.Contains(where, "-") {
		query = strings.Split(where, " ")
	} else {
		query = []string{where}
	}

	out, err := tablehelpers.RunSimple(ctx, t.slogger, 120, allowedcmd.Mdfind, query)
	if err != nil {
		return nil, fmt.Errorf("call mdfind: %w", err)
	}

	var resp []map[string]string

	lr := bufio.NewReader(bytes.NewReader(out))
	for {
		line, _, err := lr.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		m := make(map[string]string, 2)
		m["query"] = where
		m["path"] = string(line)
		resp = append(resp, m)
	}

	return resp, nil
}
