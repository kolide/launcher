package table

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"strings"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func GDrivePlugin(client *osquery.ExtensionManagerClient) *table.Plugin {
	t := &gDriveTable{client: client}
	paths, err := queryDbPath(t.client)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer db.Close()

	g := &gdrive{
		db: database,
	}

	columns := []table.ColumnDefinition{
		table.TextColumn("user_email"),
		table.TextColumn("local_sync_root_path"),
	}
	return table.NewPlugin("kolide_gdrive_sync_config", columns, g.generate)
}

type gdrive struct {
	db *sql.DB
}

// GdriveGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (g *gdrive) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	rows, _ := g.db.Query("SELECT entry_key, data_key, data_value FROM data")
	defer rows.Close()
	var email string
	var localsyncpath string
	for rows.Next() {
		var entry_key string
		var data_key string
		var data_value string
		rows.Scan(&entry_key, &data_key, &data_value)

		switch entry_key {
		case "user_email":
			email = data_value
		case "local_sync_root_path":
			localsyncpath = data_value
		default:
			continue
		}
	}
	return []map[string]string{
		{
			"user_email":           email,
			"local_sync_root_path": localsyncpath,
		},
	},nil
}

func queryDbPath(client *osquery.ExtensionManagerClient) (string, error) {
	query := `select username from last where username not in ('', 'root') group by username order by count(username) desc limit 1`
	row, err := client.QueryRow(query)
	if err != nil {
		return "", errors.Wrap(err, "querying for primaryUser version")
	}
	var username string
	if val, ok := row["username"]; ok {
		username = val
	}
	path := filepath.Join("/Users", username,"/Library/Application Support/Google/Drive/user_default/sync_config.db")
	return path, nil
}