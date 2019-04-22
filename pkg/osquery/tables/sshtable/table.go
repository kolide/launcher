package sshtable

import (
	"context"
	"io/ioutil"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

type tableExtension struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

type rowData struct {
	Path      string
	Type      string
	Encrypted bool
}

// New returns a new table extesion
func New(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("type"),
		table.TextColumn("encrypted"),
	}

	t := &tableExtension{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_ssh_key_information", columns, t.generate)
}

func (t *tableExtension) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	// we require a path. We don't bother checking the type of
	// constraint. We don't check ok, since we're checking the length of
	// the return.
	q, _ := queryContext.Constraints["path"]
	if len(q.Constraints) == 0 {
		return results, errors.New("The kolide_ssh_key_information table requires that you specify a constraint on path")
	}

	for _, constraint := range q.Constraints {
		path := constraint.Expression
		r, err := t.checkFile(path)
		if err != nil {
			level.Info(t.logger).Log(
				"msg", "Error parsing key",
				"path", path,
				"err", err,
			)
			continue
		}
		results = append(results, r)
	}

	return results, nil
}

// checkFile looks at an ssh key, and determinds various things about
// it. As it's buried deep in osquery, this is expected to handle an
// error, and return an empty row.
func (t *tableExtension) checkFile(path string) (map[string]string, error) {
	keyBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "read key")
	}

	// Try to parse the key
	key, err := ssh.ParseRawPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	_ = key

	result := map[string]string{
		"path":      path,
		"type":      "yo",
		"encrypted": "0",
	}
	return result, nil
}
