package osquery

import (
	"context"
	"log"

	"github.com/kolide/osquery-go"
)

type ExampleLogger struct{}

func (f *ExampleLogger) Name() string {
	return "example_logger"
}

func (f *ExampleLogger) LogString(ctx context.Context, typ osquery.LogType, logText string) error {
	var typeString string
	switch typ {
	case osquery.LogTypeString:
		typeString = "string"
	case osquery.LogTypeSnapshot:
		typeString = "snapshot"
	case osquery.LogTypeHealth:
		typeString = "health"
	case osquery.LogTypeInit:
		typeString = "init"
	case osquery.LogTypeStatus:
		typeString = "status"
	default:
		typeString = "unknown"
	}

	log.Printf("%s: %s\n", typeString, logText)
	return nil
}

type ExampleConfig struct{}

func (f *ExampleConfig) Name() string {
	return "example_config"
}

func (f *ExampleConfig) GenerateConfigs(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"config1": `
{
  "options": {
    "host_identifier": "hostname",
    "schedule_splay_percent": 10
  },
  "schedule": {
    "macos_kextstat": {
      "query": "SELECT * FROM kernel_extensions;",
      "interval": 10
    },
    "foobar": {
      "query": "SELECT foo, bar, pid FROM foobar_table;",
      "interval": 600
    }
  }
}
`,
	}, nil
}

type ExampleTable struct{}

func (f *ExampleTable) Name() string {
	return "example_table"
}

func (f *ExampleTable) Columns() []osquery.ColumnDefinition {
	return []osquery.ColumnDefinition{
		osquery.TextColumn("text"),
		osquery.IntegerColumn("integer"),
		osquery.BigIntColumn("big_int"),
		osquery.DoubleColumn("double"),
	}
}

func (f *ExampleTable) Generate(ctx context.Context, queryContext osquery.QueryContext) ([]map[string]string, error) {
	return []map[string]string{
		{
			"text":    "hello world",
			"integer": "123",
			"big_int": "-1234567890",
			"double":  "3.14159",
		},
	}, nil
}
