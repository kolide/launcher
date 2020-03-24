package wmitable

import (
	"context"
	"log"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("query"),
		table.TextColumn("namespace"),

		table.TextColumn("key"),
		table.TextColumn("value"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_wmi", columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	queryQ, ok := queryContext.Constraints["query"]
	if !ok || len(queryQ.Constraints) == 0 {
		return results, errors.New("The kolide_wmi table requires a query")
	}
	for _, queryConstraint := range queryQ.Constraints {
		queryConstraint.Expression
	}

	namespaceQ, ok := queryContext.Constraints["namespace"]
	if ok && len(pathQ.Constraints) > 0 {
		for _, nsConstraint := range namespaceQ.Constraints {
			namespace := nsConstraint.Expression

			wmi.QueryNamespace(query, namespace)
		}
	}

}

func wmiQuery(query string, namespace string) ([]map[string]string, error) {
	var results []map[string]string
	var wmiResults []interface{}

	if namespace != "" {
		if err := wmi.Query(query, wmiResults); err != nil {
			return nil, errors.Wrap(err, "wmi query")
		}
	} else {
		if err := wmi.QueryNamespace(query, wmiResults, namespace); err != nil {
			return nil, errors.Wrapf(err, "wmi query in namespace %s", namespace)
		}
	}

	// parse the results

}
