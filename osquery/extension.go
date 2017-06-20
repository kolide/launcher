package osquery

import (
	"context"
	"log"

	"github.com/kolide/osquery-go/plugin/logger"
)

func GenerateConfigs(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"config1": `
{
  "options": {
    "host_identifier": "hostname"
  },
  "schedule": {
    "time": {
      "query": "select * from time;",
      "interval": 2
    },
		"osquery_info": {
			"query": "select * from osquery_info;",
			"interval": 2,
			"snapshot": true
		}
  }
}
`,
	}, nil
}

func LogString(ctx context.Context, typ logger.LogType, logText string) error {
	log.Printf("%s: %s\n", typ, logText)
	return nil
}
