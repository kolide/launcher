package tablehelpers

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func ExecOsqueryGetColumns(ctx context.Context, logger log.Logger, osqueryPath string, tableName string) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(5)*time.Second)
	defer cancel()

	// FIXME: Allowed characters
	query := fmt.Sprintf("select sql from sqlite_temp_master where name = '%s'", tableName)

	cmd := exec.CommandContext(ctx,
		osqueryPath,
		"--config_path", "/dev/null",
		"--disable_events",
		"--disable_database",
		"--disable_audit",
		"--ephemeral",
		"-S",
		"--json",
		query,
	)
	_ = cmd

	dir, err := ioutil.TempDir("", "osq-schema")
	if err != nil {
		//return nil, errors.Wrap(err, "mktemp")
	}
	defer os.RemoveAll(dir)

}

func parseSqlCreate(sqlCreate string) ([]table.ColumnDefinition, error) {
	openParen := strings.Index(sqlCreate, "(")
	if openParen < 1 {
		return nil, errors.New("Invalid string. No opening paren")
	}

	closeParen := strings.LastIndex(sqlCreate, ")")
	if closeParen < 1 {
		return nil, errors.New("Invalid string. No closing paren")
	}

	// strip off the openParen as well
	openParen++

	if openParen > len(sqlCreate) || closeParen > len(sqlCreate) {
		return nil, errors.New("parens out of bounds")
	}
	columnCreates := sqlCreate[openParen:closeParen]

	columnDefs := strings.Split(columnCreates, ",")

	columns := make([]table.ColumnDefinition, len(columnDefs))

	for i, columnDef := range columnDefs {
		components := strings.Split(columnDef, "`")
		if len(components) != 3 {
			return nil, errors.Errorf("Unable to parse '%s' as column definition", columnDef)
		}

		name := components[1]
		cType := strings.Trim(components[2], " ")

		switch cType {
		case "TEXT":
			columns[i] = table.TextColumn(name)
		case "INTEGER":
			columns[i] = table.IntegerColumn(name)
		default:
			return nil, errors.Errorf("Unknown column type '%s'", cType)
		}

		fmt.Println(name, cType)
	}

	return columns, nil
}
